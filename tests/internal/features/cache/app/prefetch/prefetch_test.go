// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package prefetch_test

// Verifies:
//   - A prefetch of a multi-platform image fetches only the variants of the platforms asked, each
//     blob once, and reports done with its counts
//   - With no platform asked, every variant is fetched
//   - An image that cannot be found fails the job and says which
//   - A malformed image or platform refuses the request; an unknown job is not found

import (
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/adapters/registry/upstream"
	"github.com/nauarch/limen/internal/adapters/storage/disk"
	contract "github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/features/cache/app/fetch"
	"github.com/nauarch/limen/internal/features/cache/app/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	"github.com/nauarch/limen/tests/kit/registry"
)

// setup is a prefetcher over a kit registry holding a two-platform image vllm/vllm-openai:v0.32.0
// whose variants share one base layer.
func setup(t *testing.T) (*prefetch.Prefetcher, *registry.Registry) {
	t.Helper()
	reg := registry.New(registry.Options{})
	t.Cleanup(reg.Close)
	shared := reg.PutBlob([]byte("shared base layer"))
	var children string
	for i, arch := range []string{"amd64", "arm64"} {
		cfg := reg.PutBlob([]byte("config " + arch))
		own := reg.PutBlob([]byte("layer " + arch))
		body := `{"schemaVersion":2,"config":{"digest":"` + cfg.String() + `","size":1},"layers":[` +
			`{"digest":"` + shared.String() + `","size":1},{"digest":"` + own.String() + `","size":1}]}`
		d := reg.PutManifest("vllm/vllm-openai", "", "application/vnd.oci.image.manifest.v1+json", []byte(body))
		if i > 0 {
			children += ","
		}
		children += `{"digest":"` + d.String() + `","platform":{"os":"linux","architecture":"` + arch + `"}}`
	}
	reg.PutManifest("vllm/vllm-openai", "v0.32.0", "application/vnd.oci.image.index.v1+json", []byte(`{"schemaVersion":2,"manifests":[`+children+`]}`))

	store, err := disk.Open(disk.Options{Root: t.TempDir(), Init: true})
	if err != nil {
		t.Fatal(err)
	}
	up := upstream.New(upstream.Options{Endpoints: map[string]string{"docker.io": reg.URL()}, Timeout: 5 * time.Second})
	clk := kitclock.At(time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC))
	counters, log := &fetch.Counters{}, slog.New(slog.DiscardHandler)
	return &prefetch.Prefetcher{
		Manifests:  &fetch.Manifests{Store: store, Upstream: up, Clock: clk, TagTTL: -1, AnswerWithin: 5 * time.Second, Counters: counters, Log: log},
		Blobs:      &fetch.Blobs{Store: store, Upstream: up, Background: t.Context(), Clock: clk, AnswerWithin: 5 * time.Second, StallAfter: 5 * time.Second, Counters: counters, Log: log},
		Background: t.Context(),
		Clock:      clk,
		Parallel:   2,
		Log:        log,
	}, reg
}

// finish waits for a job to end.
func finish(t *testing.T, p *prefetch.Prefetcher, id string) contract.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		j, err := p.Job(id)
		if err != nil {
			t.Fatal(err)
		}
		if j.State != contract.Running {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job still running: %+v", j)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOnlyTheAskedPlatformsAreFetched(t *testing.T) {
	p, reg := setup(t)
	j, err := p.Start(t.Context(), contract.Request{Images: []string{"vllm/vllm-openai:v0.32.0"}, Platforms: []string{"linux/arm64"}})
	if err != nil {
		t.Fatal(err)
	}
	got := finish(t, p, j.ID)
	if got.State != contract.Done || got.Images != 1 || got.Blobs != 3 || got.Fetched != 3 || got.Bytes == 0 {
		t.Fatalf("%+v", got)
	}
	if n := reg.Count(registry.GetBlob); n != 3 {
		t.Errorf("upstream blob GETs: %d", n)
	}
}

func TestEveryVariantIsFetchedWhenNoPlatformIsAsked(t *testing.T) {
	p, _ := setup(t)
	j, err := p.Start(t.Context(), contract.Request{Images: []string{"docker.io/vllm/vllm-openai:v0.32.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := finish(t, p, j.ID); got.State != contract.Done || got.Blobs != 5 {
		t.Fatalf("shared layer counted once: want 5 blobs, got %+v", got)
	}
}

func TestAMissingImageFailsTheJob(t *testing.T) {
	p, _ := setup(t)
	j, err := p.Start(t.Context(), contract.Request{Images: []string{"vllm/vllm-openai:v0.0.1", "vllm/vllm-openai:v0.32.0"}, Platforms: []string{"linux/amd64"}})
	if err != nil {
		t.Fatal(err)
	}
	got := finish(t, p, j.ID)
	if got.State != contract.Failed || len(got.Failures) != 1 || got.Fetched != 3 {
		t.Fatalf("%+v", got)
	}
}

func TestAMalformedRequestIsRefused(t *testing.T) {
	p, _ := setup(t)
	for i, r := range []contract.Request{
		{Images: []string{"Not An Image"}},
		{Images: []string{"nginx"}, Platforms: []string{"linux"}},
	} {
		if _, err := p.Start(t.Context(), r); errkind.Of(err) != errkind.Invalid {
			t.Errorf("request %s: %v", strconv.Itoa(i), err)
		}
	}
	if _, err := p.Job("nope"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("unknown job: %v", err)
	}
}
