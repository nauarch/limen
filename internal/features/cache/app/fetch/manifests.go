// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/contracts/storage"
	"github.com/nauarch/limen/internal/contracts/upstream"
	"github.com/nauarch/limen/internal/features/cache/domain/policy"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Manifests hands out manifests. One named by digest never changes, so it is served from the
// store whenever it is there. A tag asked within TagTTL of its last check is served as it was;
// after that it is asked upstream, by HEAD first so an unchanged tag costs no pull. Upstream gets
// AnswerWithin to answer; when it fails or is too slow, the tag's last known manifest is served.
type Manifests struct {
	Store        storage.Store
	Upstream     upstream.Upstream
	Clock        clock.Clock
	TagTTL       time.Duration
	AnswerWithin time.Duration
	Counters     *Counters
	Log          *slog.Logger
}

// Get is a repository's manifest by tag or digest.
func (m *Manifests) Get(ctx context.Context, registry, repository, reference string, accept []string) (pull.Manifest, error) {
	ref, err := policy.ParseReference(reference)
	if err != nil {
		return pull.Manifest{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, m.AnswerWithin)
	defer cancel()
	if ref.Digest != "" {
		return m.byDigest(ctx, registry, repository, ref.Digest, accept)
	}
	return m.byTag(ctx, registry, repository, ref.Tag, accept)
}

// byDigest serves a stored manifest, or fetches, checks and stores it.
func (m *Manifests) byDigest(ctx context.Context, registry, repository string, d digest.Digest, accept []string) (pull.Manifest, error) {
	if got, ok, err := m.stored(d); ok || err != nil {
		return got, err
	}
	got, err := m.Upstream.Manifest(ctx, registry, repository, d.String(), accept)
	if err != nil {
		return pull.Manifest{}, err
	}
	if digest.Of(got.Body) != d {
		return pull.Manifest{}, fmt.Errorf("manifest %s of %s/%s: upstream sent other content: %w", d, registry, repository, errkind.ErrUpstream)
	}
	got.Digest = d
	m.keep(got)
	return got, nil
}

// byTag serves a tag checked within its lifetime as it was; otherwise it asks upstream what the
// tag points at and serves that, falling back to the tag's last known manifest when upstream
// fails.
func (m *Manifests) byTag(ctx context.Context, registry, repository, tag string, accept []string) (pull.Manifest, error) {
	rec, recErr := m.Store.Tag(registry, repository, tag)
	if recErr == nil && policy.Fresh(rec.Fetched, m.Clock.Now(), m.TagTTL) {
		if got, ok, serr := m.stored(rec.Digest); serr == nil && ok {
			m.Counters.TagsFresh.Add(1)
			return got, nil
		}
	}
	m.Counters.TagsChecked.Add(1)
	d, _, err := m.Upstream.Resolve(ctx, registry, repository, tag, accept)
	if err == nil {
		if got, ok, serr := m.stored(d); serr == nil && ok {
			m.record(registry, repository, tag, got)
			return got, nil
		}
	}
	// A HEAD that answered without a usable digest is retried as a GET.
	if err == nil || errkind.Of(err) == errkind.Upstream {
		got, gerr := m.Upstream.Manifest(ctx, registry, repository, tag, accept)
		if gerr == nil {
			got.Digest = digest.Of(got.Body)
			m.keep(got)
			m.record(registry, repository, tag, got)
			return got, nil
		}
		err = gerr
	}
	if !policy.ServeStale(err) || recErr != nil {
		return pull.Manifest{}, err
	}
	got, ok, serr := m.stored(rec.Digest)
	if serr != nil || !ok {
		return pull.Manifest{}, err
	}
	m.Counters.TagsStale.Add(1)
	m.Log.Warn("serving a stale tag", "registry", registry, "repository", repository, "tag", tag, "digest", got.Digest, "fetched", rec.Fetched, "err", err)
	return got, nil
}

// stored reads a manifest from the store; ok is false when it is not there.
func (m *Manifests) stored(d digest.Digest) (pull.Manifest, bool, error) {
	mediaType, body, err := m.Store.Manifest(d)
	switch {
	case err == nil:
		return pull.Manifest{MediaType: mediaType, Digest: d, Body: body}, true, nil
	case errkind.Of(err) == errkind.NotFound:
		return pull.Manifest{}, false, nil
	default:
		return pull.Manifest{}, false, err
	}
}

// keep stores a manifest; a store failure is logged, since the manifest can still be served.
func (m *Manifests) keep(got pull.Manifest) {
	if err := m.Store.PutManifest(got.Digest, got.MediaType, got.Body); err != nil {
		m.Log.Error("manifest not stored", "digest", got.Digest, "err", err)
	}
}

// record notes what a tag points at now.
func (m *Manifests) record(registry, repository, tag string, got pull.Manifest) {
	r := storage.TagRecord{Digest: got.Digest, MediaType: got.MediaType, Fetched: m.Clock.Now()}
	if err := m.Store.PutTag(registry, repository, tag, r); err != nil {
		m.Log.Error("tag not recorded", "registry", registry, "repository", repository, "tag", tag, "err", err)
	}
}
