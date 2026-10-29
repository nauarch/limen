// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/nauarch/limen/internal/contracts/ops"
	contract "github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/contracts/storage"
	"github.com/nauarch/limen/internal/contracts/upstream"
	"github.com/nauarch/limen/internal/features/cache/app/evict"
	"github.com/nauarch/limen/internal/features/cache/app/fetch"
	"github.com/nauarch/limen/internal/features/cache/app/prefetch"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/kernel/image"
)

// The defaults of the cache's timing.
const (
	// DefaultTagTTL is how long a checked tag is answered without asking upstream again.
	DefaultTagTTL = time.Minute
	// DefaultAnswerWithin is how long a pull waits for upstream before failing over: well under
	// the ~30 s clients wait for headers, so a slow upstream never makes them wait on limen too.
	DefaultAnswerWithin = 15 * time.Second
	// DefaultStallAfter is how long a download may receive nothing before it is given up.
	DefaultStallAfter = time.Minute
	// DefaultEvictEvery is how often the store is measured and, over its cap, trimmed.
	DefaultEvictEvery = 5 * time.Minute
	// DefaultPrefetchParallel is how many blobs a prefetch fetches at once.
	DefaultPrefetchParallel = 4
)

// Options are what the cache is built from: where it keeps things, where it pulls them from,
// which registries it serves (by name, such as docker.io or ghcr.io), its timing (zero takes the
// defaults above), the store's cap in bytes (0 for none), the context its downloads run on (they
// stop when it ends), the clock and the log.
type Options struct {
	Store        storage.Store
	Upstream     upstream.Upstream
	Registries   []string
	TagTTL       time.Duration
	AnswerWithin time.Duration
	StallAfter   time.Duration
	MaxBytes     int64
	EvictEvery   time.Duration
	Background   context.Context
	Clock        clock.Clock
	Log          *slog.Logger
}

// Cache is the pull-through cache: a pull.Puller for the mirror, a prefetch.Prefetcher and an
// ops.Reporter for the ops door.
type Cache struct {
	store      storage.Store
	registries map[string]bool
	counters   *fetch.Counters
	blobs      *fetch.Blobs
	manifests  *fetch.Manifests
	evictor    *evict.Evictor
	prefetcher *prefetch.Prefetcher
	evictEvery time.Duration
}

var (
	_ pull.Puller         = (*Cache)(nil)
	_ contract.Prefetcher = (*Cache)(nil)
	_ ops.Reporter        = (*Cache)(nil)
)

// Compose builds the cache. A store and an upstream are required.
func Compose(o Options) (*Cache, error) {
	if o.Store == nil || o.Upstream == nil {
		return nil, fmt.Errorf("cache: a store and an upstream are required: %w", errkind.ErrInvalid)
	}
	o = withDefaults(o)
	served := map[string]bool{}
	for _, r := range o.Registries {
		served[image.Canonical(r)] = true
	}
	counters := &fetch.Counters{}
	blobs := &fetch.Blobs{
		Store: o.Store, Upstream: o.Upstream, Background: o.Background, Clock: o.Clock,
		AnswerWithin: o.AnswerWithin, StallAfter: o.StallAfter, Counters: counters, Log: o.Log,
	}
	manifests := &fetch.Manifests{
		Store: o.Store, Upstream: o.Upstream, Clock: o.Clock,
		TagTTL: o.TagTTL, AnswerWithin: o.AnswerWithin, Counters: counters, Log: o.Log,
	}
	return &Cache{
		store:      o.Store,
		registries: served,
		counters:   counters,
		blobs:      blobs,
		manifests:  manifests,
		evictor:    &evict.Evictor{Store: o.Store, Max: o.MaxBytes, Log: o.Log},
		prefetcher: &prefetch.Prefetcher{
			Manifests: manifests, Blobs: blobs, Background: o.Background, Clock: o.Clock,
			Parallel: DefaultPrefetchParallel, Log: o.Log,
		},
		evictEvery: o.EvictEvery,
	}, nil
}

// withDefaults fills what the options leave empty.
func withDefaults(o Options) Options {
	if o.Background == nil {
		o.Background = context.Background()
	}
	if o.Clock == nil {
		o.Clock = clock.System{}
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.TagTTL == 0 {
		o.TagTTL = DefaultTagTTL
	}
	if o.AnswerWithin <= 0 {
		o.AnswerWithin = DefaultAnswerWithin
	}
	if o.StallAfter <= 0 {
		o.StallAfter = DefaultStallAfter
	}
	if o.EvictEvery <= 0 {
		o.EvictEvery = DefaultEvictEvery
	}
	return o
}

// Maintain measures the store and keeps it under its cap until ctx ends.
func (c *Cache) Maintain(ctx context.Context) { c.evictor.Run(ctx, c.evictEvery) }

// Serves reports whether a registry is on the cache's list.
func (c *Cache) Serves(registry string) bool { return c.registries[image.Canonical(registry)] }

// Manifest is a repository's manifest by tag or digest.
func (c *Cache) Manifest(ctx context.Context, registry, repository, reference string, accept []string) (pull.Manifest, error) {
	registry, err := c.check(registry, repository)
	if err != nil {
		return pull.Manifest{}, err
	}
	return c.manifests.Get(ctx, registry, repository, reference, accept)
}

// Blob is a repository's blob by digest.
func (c *Cache) Blob(ctx context.Context, registry, repository string, d digest.Digest) (pull.Blob, error) {
	registry, err := c.check(registry, repository)
	if err != nil {
		return pull.Blob{}, err
	}
	return c.blobs.Get(ctx, registry, repository, d)
}

// Start starts a prefetch job; an image from a registry the cache does not serve refuses it.
func (c *Cache) Start(ctx context.Context, r contract.Request) (contract.Job, error) {
	for _, s := range r.Images {
		ref, err := image.Parse(s)
		if err != nil {
			return contract.Job{}, err
		}
		if !c.Serves(ref.Registry) {
			return contract.Job{}, fmt.Errorf("image %s: registry %s is not served: %w", s, ref.Registry, errkind.ErrForbidden)
		}
	}
	return c.prefetcher.Start(ctx, r)
}

// Job is a prefetch job's progress.
func (c *Cache) Job(id string) (contract.Job, error) { return c.prefetcher.Job(id) }

// Registries are the registries the cache serves, sorted.
func (c *Cache) Registries() []string {
	out := make([]string, 0, len(c.registries))
	for r := range c.registries {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// Healthy fails when the store cannot be written to.
func (c *Cache) Healthy() error { return c.store.Check() }

// Stats are the cache's counters.
func (c *Cache) Stats() ops.Stats {
	k, e := c.counters, c.evictor
	return ops.Stats{
		BlobHits: k.BlobHits.Load(), BlobJoins: k.BlobJoins.Load(), BlobMisses: k.BlobMisses.Load(),
		BlobFailures: k.BlobFailures.Load(), BytesServed: k.BytesServed.Load(), BytesFetched: k.BytesFetched.Load(),
		TagsFresh: k.TagsFresh.Load(), TagsChecked: k.TagsChecked.Load(), TagsStale: k.TagsStale.Load(),
		Evicted: e.Evicted.Load(), EvictedBytes: e.EvictedBytes.Load(),
		StoreBytes: e.StoreBytes.Load(), StoreBlobs: e.StoreBlobs.Load(), MaxBytes: e.Max,
	}
}

// check refuses a registry the cache does not serve and a malformed repository name, and gives
// the registry's canonical name.
func (c *Cache) check(registry, repository string) (string, error) {
	registry = image.Canonical(registry)
	if !c.registries[registry] {
		return "", fmt.Errorf("registry %s is not served: %w", registry, errkind.ErrForbidden)
	}
	if !image.ValidRepository(repository) {
		return "", fmt.Errorf("repository %q: %w", repository, errkind.ErrInvalid)
	}
	return registry, nil
}
