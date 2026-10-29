// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package prefetch

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	contract "github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/features/cache/app/fetch"
	"github.com/nauarch/limen/internal/features/cache/domain/oci"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/kernel/image"
)

// keptJobs is how many jobs are remembered; older ones are forgotten.
const keptJobs = 100

// Prefetcher warms the cache: for each image it walks the manifest (and, for a multi-platform
// image, the variants of the platforms asked for) and pulls every blob into the store, Parallel at
// a time, through the same fetchers pulls use, so it joins downloads already running. Jobs run on
// Background.
type Prefetcher struct {
	Manifests  *fetch.Manifests
	Blobs      *fetch.Blobs
	Background context.Context
	Clock      clock.Clock
	Parallel   int
	Log        *slog.Logger

	mu    sync.Mutex
	jobs  map[string]*job
	order []string
}

// Start checks a request and starts its job; a malformed image or platform refuses the whole
// request before anything is fetched.
func (p *Prefetcher) Start(_ context.Context, r contract.Request) (contract.Job, error) {
	refs := make([]image.Ref, 0, len(r.Images))
	for _, s := range r.Images {
		ref, err := image.Parse(s)
		if err != nil {
			return contract.Job{}, err
		}
		refs = append(refs, ref)
	}
	var platforms []oci.Platform
	for _, s := range r.Platforms {
		pl, err := oci.ParsePlatform(s)
		if err != nil {
			return contract.Job{}, err
		}
		platforms = append(platforms, pl)
	}
	id, err := newID()
	if err != nil {
		return contract.Job{}, err
	}
	j := &job{pj: contract.Job{ID: id, State: contract.Running, Images: len(refs), StartedAt: p.Clock.Now().UTC().Format(time.RFC3339)}}
	p.remember(id, j)
	go p.run(j, refs, platforms)
	return j.snapshot(), nil
}

// Job is a job's progress.
func (p *Prefetcher) Job(id string) (contract.Job, error) {
	p.mu.Lock()
	j, ok := p.jobs[id]
	p.mu.Unlock()
	if !ok {
		return contract.Job{}, fmt.Errorf("prefetch job %q: %w", id, errkind.ErrNotFound)
	}
	return j.snapshot(), nil
}

// remember keeps a job, forgetting the oldest beyond keptJobs.
func (p *Prefetcher) remember(id string, j *job) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.jobs == nil {
		p.jobs = map[string]*job{}
	}
	p.jobs[id] = j
	p.order = append(p.order, id)
	for len(p.order) > keptJobs {
		delete(p.jobs, p.order[0])
		p.order = p.order[1:]
	}
}

// target is a blob to fetch and the repository to fetch it from.
type target struct {
	registry, repository string
	digest               digest.Digest
}

// run walks every image, then fetches every blob found once.
func (p *Prefetcher) run(j *job, refs []image.Ref, platforms []oci.Platform) {
	seen := map[digest.Digest]bool{}
	var targets []target
	for _, ref := range refs {
		blobs, err := p.walk(ref, platforms)
		if err != nil {
			j.failed(ref.String() + ": " + err.Error())
			continue
		}
		for _, b := range blobs {
			if !seen[b] {
				seen[b] = true
				targets = append(targets, target{registry: ref.Registry, repository: ref.Repository, digest: b})
			}
		}
	}
	j.found(len(targets))
	sem := make(chan struct{}, max(p.Parallel, 1))
	var wg sync.WaitGroup
	for _, t := range targets {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			n, err := p.fetch(t)
			if err != nil {
				j.failed(t.digest.String() + ": " + err.Error())
				return
			}
			j.fetched(n)
		})
	}
	wg.Wait()
	j.end()
	s := j.snapshot()
	p.Log.Info("prefetch finished", "job", s.ID, "state", s.State, "images", s.Images, "blobs", s.Blobs, "bytes", s.Bytes, "failures", len(s.Failures))
}

// walk lists the blobs of an image: its manifest's, or for a multi-platform image those of each
// variant whose platform was asked for (every variant when none were).
func (p *Prefetcher) walk(ref image.Ref, platforms []oci.Platform) ([]digest.Digest, error) {
	m, err := p.Manifests.Get(p.Background, ref.Registry, ref.Repository, ref.Reference(), oci.Manifests)
	if err != nil {
		return nil, err
	}
	parsed, err := oci.Parse(m.Body)
	if err != nil {
		return nil, err
	}
	if len(parsed.Children) == 0 {
		return blobDigests(parsed), nil
	}
	var out []digest.Digest
	for _, c := range parsed.Children {
		if !wanted(c.Platform, platforms) {
			continue
		}
		cm, err := p.Manifests.Get(p.Background, ref.Registry, ref.Repository, c.Digest.String(), oci.Manifests)
		if err != nil {
			return nil, err
		}
		child, err := oci.Parse(cm.Body)
		if err != nil {
			return nil, err
		}
		out = append(out, blobDigests(child)...)
	}
	return out, nil
}

// wanted reports whether a variant is for one of the platforms asked; with none asked, all are.
func wanted(pl oci.Platform, platforms []oci.Platform) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, want := range platforms {
		if pl.Matches(want) {
			return true
		}
	}
	return false
}

// blobDigests lists an image manifest's blobs.
func blobDigests(parsed oci.Parsed) []digest.Digest {
	out := make([]digest.Digest, 0, len(parsed.Blobs))
	for _, b := range parsed.Blobs {
		out = append(out, b.Digest)
	}
	return out
}

// fetch pulls one blob into the store by reading it through, and tells its size.
func (p *Prefetcher) fetch(t target) (int64, error) {
	b, err := p.Blobs.Get(p.Background, t.registry, t.repository, t.digest)
	if err != nil {
		return 0, err
	}
	defer b.Body.Close()
	return io.Copy(io.Discard, b.Body)
}

// newID is a random job ID.
func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("prefetch: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
