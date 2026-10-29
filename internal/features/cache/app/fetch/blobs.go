// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/contracts/storage"
	"github.com/nauarch/limen/internal/contracts/upstream"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Blobs hands out blobs from the store, or from one download that every concurrent pull of the
// same blob shares. Downloads run on Background, not on the pull that started them, so a blob
// whose first puller goes away still lands for the next; canceling Background stops them all.
//
// A pull is answered within AnswerWithin: when upstream has not answered by then, the pull fails
// as unreachable (so its client falls back to the real registry) while the download goes on. A
// download that receives nothing for StallAfter is given up.
type Blobs struct {
	Store        storage.Store
	Upstream     upstream.Upstream
	Background   context.Context
	Clock        clock.Clock
	AnswerWithin time.Duration
	StallAfter   time.Duration
	Counters     *Counters
	Log          *slog.Logger

	mu      sync.Mutex
	flights map[digest.Digest]*flight
}

// Get is a blob: from the store when it is there, else joined to its download, which it starts
// when none runs. It returns once upstream has answered, so the size is known.
func (b *Blobs) Get(ctx context.Context, registry, repository string, d digest.Digest) (pull.Blob, error) {
	f, blob, err := b.flightFor(registry, repository, d)
	if f == nil {
		return blob, err
	}
	answer := time.NewTimer(b.AnswerWithin)
	defer answer.Stop()
	select {
	case <-f.started:
	case <-ctx.Done():
		return pull.Blob{}, ctx.Err()
	case <-answer.C:
		return pull.Blob{}, fmt.Errorf("blob %s: upstream has not answered within %s: %w", d, b.AnswerWithin, errkind.ErrUnreachable)
	}
	if f.startErr != nil {
		return pull.Blob{}, f.startErr
	}
	return pull.Blob{Size: f.size, Body: &flightReader{f: f, store: b.Store, digest: d, served: &b.Counters.BytesServed}}, nil
}

// flightFor finds the running download of a blob, or the stored blob, or starts the download. The
// store is looked at under the same lock that lands a download, so a blob is never missed between
// the two.
func (b *Blobs) flightFor(registry, repository string, d digest.Digest) (*flight, pull.Blob, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if f, ok := b.flights[d]; ok {
		b.Counters.BlobJoins.Add(1)
		return f, pull.Blob{}, nil
	}
	body, size, err := b.Store.Blob(d)
	switch {
	case err == nil:
		b.Counters.BlobHits.Add(1)
		if terr := b.Store.Touch(d, b.Clock.Now()); terr != nil {
			b.Log.Debug("blob use not recorded", "digest", d, "err", terr)
		}
		return nil, pull.Blob{Size: size, Body: countedBlob{ReadSeekCloser: body, served: &b.Counters.BytesServed}}, nil
	case errkind.Of(err) != errkind.NotFound:
		return nil, pull.Blob{}, err
	}
	if b.flights == nil {
		b.flights = map[digest.Digest]*flight{}
	}
	b.Counters.BlobMisses.Add(1)
	f := newFlight()
	b.flights[d] = f
	go b.download(f, registry, repository, d)
	return f, pull.Blob{}, nil
}

// download fetches a blob into the store for its flight, then lets the flight go.
func (b *Blobs) download(f *flight, registry, repository string, d digest.Digest) {
	defer b.land(d)
	body, size, err := b.Upstream.Blob(b.Background, registry, repository, d)
	if err != nil {
		b.Counters.BlobFailures.Add(1)
		f.fail(err)
		b.Log.Warn("blob not fetched", "registry", registry, "repository", repository, "digest", d, "err", err)
		return
	}
	defer body.Close()
	pending, err := b.Store.Begin(d)
	if err != nil {
		b.Counters.BlobFailures.Add(1)
		f.fail(err)
		b.Log.Error("blob not stored", "digest", d, "err", err)
		return
	}
	f.begin(size, pending)
	err = f.fill(body, d, b.StallAfter, &b.Counters.BytesFetched)
	f.finish(err)
	if err != nil {
		b.Counters.BlobFailures.Add(1)
		b.Log.Warn("blob download failed", "registry", registry, "repository", repository, "digest", d, "err", err)
		return
	}
	b.Log.Info("blob cached", "registry", registry, "repository", repository, "digest", d, "size", size)
}

// land forgets a finished download; later pulls find the blob in the store.
func (b *Blobs) land(d digest.Digest) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.flights, d)
}
