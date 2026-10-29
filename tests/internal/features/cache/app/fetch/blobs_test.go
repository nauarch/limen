// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package fetch_test

// Verifies:
//   - Pulls of one blob that arrive while it downloads share a single upstream download, and each
//     gets every byte
//   - Once landed, a blob is served from the store without asking upstream
//   - A blob whose bytes do not match its digest fails every pull and is not kept
//   - A pull that gives up does not stop the download: the blob still lands for the next
//   - A pull is answered in time when upstream is slow (unreachable, so the client falls back),
//     and the download still lands
//   - A download that stops sending is given up, fails its pulls and keeps nothing

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/adapters/registry/upstream"
	"github.com/nauarch/limen/internal/adapters/storage/disk"
	"github.com/nauarch/limen/internal/features/cache/app/fetch"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	"github.com/nauarch/limen/tests/kit/registry"
)

// rig is a cache's fetchers over a real store and a kit registry standing in for docker.io, on a
// fixed clock. Tags are asked upstream every time unless a test gives them a lifetime.
type rig struct {
	reg       *registry.Registry
	store     *disk.Dir
	clock     *kitclock.Fixed
	counters  *fetch.Counters
	blobs     *fetch.Blobs
	manifests *fetch.Manifests
}

// newRig builds a rig; the registry is closed when the test ends.
func newRig(t *testing.T, o registry.Options) *rig {
	t.Helper()
	reg := registry.New(o)
	t.Cleanup(reg.Close)
	store, err := disk.Open(disk.Options{Root: t.TempDir(), Init: true})
	if err != nil {
		t.Fatal(err)
	}
	up := upstream.New(upstream.Options{Endpoints: map[string]string{"docker.io": reg.URL()}, Timeout: 5 * time.Second})
	log := slog.New(slog.DiscardHandler)
	clk := kitclock.At(time.Date(2026, 10, 29, 5, 0, 0, 0, time.UTC))
	counters := &fetch.Counters{}
	return &rig{
		reg:      reg,
		store:    store,
		clock:    clk,
		counters: counters,
		blobs: &fetch.Blobs{
			Store: store, Upstream: up, Background: t.Context(), Clock: clk,
			AnswerWithin: 3 * time.Second, StallAfter: 3 * time.Second, Counters: counters, Log: log,
		},
		manifests: &fetch.Manifests{
			Store: store, Upstream: up, Clock: clk,
			TagTTL: -1, AnswerWithin: 3 * time.Second, Counters: counters, Log: log,
		},
	}
}

// waitStored waits until a blob is in the store.
func waitStored(t *testing.T, r *rig, d digest.Digest) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if body, _, err := r.store.Blob(d); err == nil {
			_ = body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the blob never landed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// layer is a blob larger than one copy chunk.
func layer() []byte { return bytes.Repeat([]byte("limen layer bytes "), 64<<10) }

func TestConcurrentPullsShareOneDownload(t *testing.T) {
	r := newRig(t, registry.Options{RedirectBlobs: true})
	data := layer()
	d := r.reg.PutBlob(data)
	release := r.reg.Hold()
	defer release()

	const pulls = 3
	bodies := make([]io.ReadCloser, pulls)
	var joined sync.WaitGroup
	for i := range pulls {
		joined.Go(func() {
			got, err := r.blobs.Get(t.Context(), "docker.io", "vllm/vllm-openai", d)
			if err != nil {
				t.Errorf("pull %d: %v", i, err)
				return
			}
			if got.Size != int64(len(data)) {
				t.Errorf("pull %d: size %d", i, got.Size)
			}
			bodies[i] = got.Body
		})
	}
	joined.Wait()
	release()
	for i, body := range bodies {
		if body == nil {
			continue
		}
		got, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil || !bytes.Equal(got, data) {
			t.Errorf("pull %d: %d bytes, %v", i, len(got), err)
		}
	}
	if n := r.reg.Count(registry.GetBlob); n != 1 {
		t.Fatalf("upstream asked %d times for one blob", n)
	}

	again, err := r.blobs.Get(t.Context(), "docker.io", "library/other", d)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(again.Body)
	_ = again.Body.Close()
	if !bytes.Equal(got, data) || r.reg.Count(registry.GetBlob) != 1 {
		t.Errorf("from the store: %d bytes, upstream asked %d times", len(got), r.reg.Count(registry.GetBlob))
	}
}

func TestACorruptBlobFailsAndIsNotKept(t *testing.T) {
	r := newRig(t, registry.Options{})
	d := digest.Of([]byte("the wanted bytes"))
	r.reg.PutBlobAs(d, []byte("tampered bytes"))
	got, err := r.blobs.Get(t.Context(), "docker.io", "library/nginx", d)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(got.Body)
	_ = got.Body.Close()
	if errkind.Of(err) != errkind.Upstream {
		t.Fatalf("got %v, want an upstream failure", err)
	}
	if _, _, err := r.store.Blob(d); errkind.Of(err) != errkind.NotFound {
		t.Errorf("a corrupt blob was kept: %v", err)
	}
}

func TestADownloadOutlivesThePullThatStartedIt(t *testing.T) {
	r := newRig(t, registry.Options{})
	data := layer()
	d := r.reg.PutBlob(data)
	release := r.reg.Hold()
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	got, err := r.blobs.Get(ctx, "docker.io", "library/nginx", d)
	if err != nil {
		t.Fatal(err)
	}
	_ = got.Body.Close()
	cancel()
	release()
	waitStored(t, r, d)
	if n := r.reg.Count(registry.GetBlob); n != 1 {
		t.Errorf("upstream asked %d times", n)
	}
}

func TestASlowUpstreamFailsThePullOverInTime(t *testing.T) {
	r := newRig(t, registry.Options{})
	data := layer()
	d := r.reg.PutBlob(data)
	r.blobs.AnswerWithin = 200 * time.Millisecond
	release := r.reg.Stall()
	defer release()
	start := time.Now()
	_, err := r.blobs.Get(t.Context(), "docker.io", "library/nginx", d)
	if errkind.Of(err) != errkind.Unreachable {
		t.Fatalf("got %v, want unreachable", err)
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Errorf("the pull waited %s", waited)
	}
	release()
	waitStored(t, r, d)
}

func TestAStalledDownloadIsGivenUp(t *testing.T) {
	r := newRig(t, registry.Options{})
	data := layer()
	d := r.reg.PutBlob(data)
	r.blobs.StallAfter = 200 * time.Millisecond
	release := r.reg.Hold()
	defer release()
	got, err := r.blobs.Get(t.Context(), "docker.io", "library/nginx", d)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(got.Body)
	_ = got.Body.Close()
	if errkind.Of(err) != errkind.Unreachable {
		t.Fatalf("got %v, want unreachable", err)
	}
	if _, _, err := r.store.Blob(d); errkind.Of(err) != errkind.NotFound {
		t.Errorf("a stalled blob was kept: %v", err)
	}
	if r.counters.BlobFailures.Load() != 1 || r.counters.BytesFetched.Load() == 0 {
		t.Errorf("failures %d, fetched %d", r.counters.BlobFailures.Load(), r.counters.BytesFetched.Load())
	}
}
