// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package fetch_test

// Verifies:
//   - A manifest by digest is fetched once, then served from the store
//   - A tag is asked by HEAD each time; an unchanged tag costs no second GET
//   - With upstream down, a tag's last known manifest is served
//   - A tag upstream no longer has is not found, not served stale
//   - A tag within its lifetime is answered without asking upstream; after it, upstream is asked
//   - A slow upstream gets the stale tag served within the answer deadline

import (
	"testing"
	"time"

	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/tests/kit/registry"
)

// index is a manifest list's media type.
const index = "application/vnd.oci.image.index.v1+json"

func TestAManifestByDigestIsFetchedOnce(t *testing.T) {
	r := newRig(t, registry.Options{})
	d := r.reg.PutManifest("library/nginx", "", index, []byte(`{"by":"digest"}`))
	for range 2 {
		got, err := r.manifests.Get(t.Context(), "docker.io", "library/nginx", d.String(), nil)
		if err != nil || got.Digest != d || got.MediaType != index {
			t.Fatalf("%+v %v", got, err)
		}
	}
	if n := r.reg.Count(registry.GetManifest); n != 1 {
		t.Errorf("upstream GETs: %d", n)
	}
}

func TestAnUnchangedTagCostsOnlyAHead(t *testing.T) {
	r := newRig(t, registry.Options{})
	d := r.reg.PutManifest("vllm/vllm-openai", "v0.32.0", index, []byte(`{"v":32}`))
	for range 3 {
		got, err := r.manifests.Get(t.Context(), "docker.io", "vllm/vllm-openai", "v0.32.0", []string{index})
		if err != nil || got.Digest != d {
			t.Fatalf("%+v %v", got, err)
		}
	}
	if get, head := r.reg.Count(registry.GetManifest), r.reg.Count(registry.HeadManifest); get != 1 || head != 3 {
		t.Errorf("GET %d, HEAD %d", get, head)
	}
}

func TestATagIsServedStaleWhileUpstreamIsDown(t *testing.T) {
	r := newRig(t, registry.Options{})
	d := r.reg.PutManifest("library/redis", "8", index, []byte(`{"r":8}`))
	if _, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "8", nil); err != nil {
		t.Fatal(err)
	}
	r.reg.Close()
	got, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "8", nil)
	if err != nil || got.Digest != d {
		t.Fatalf("stale: %+v %v", got, err)
	}
	if _, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "never-seen", nil); errkind.Of(err) != errkind.Unreachable {
		t.Errorf("unknown tag with upstream down: %v", err)
	}
}

func TestAGoneTagIsNotServedStale(t *testing.T) {
	r := newRig(t, registry.Options{})
	r.reg.PutManifest("library/redis", "7", index, []byte(`{"r":7}`))
	if _, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "7", nil); err != nil {
		t.Fatal(err)
	}
	r.reg.DeleteTag("library/redis", "7")
	if _, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "7", nil); errkind.Of(err) != errkind.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestATagWithinItsLifetimeIsNotAskedAgain(t *testing.T) {
	r := newRig(t, registry.Options{})
	r.manifests.TagTTL = time.Minute
	r.reg.PutManifest("library/nginx", "latest", index, []byte(`{"n":1}`))
	for range 3 {
		if _, err := r.manifests.Get(t.Context(), "docker.io", "library/nginx", "latest", nil); err != nil {
			t.Fatal(err)
		}
	}
	if head := r.reg.Count(registry.HeadManifest); head != 1 {
		t.Fatalf("HEAD %d within the lifetime", head)
	}
	r.clock.Advance(2 * time.Minute)
	if _, err := r.manifests.Get(t.Context(), "docker.io", "library/nginx", "latest", nil); err != nil {
		t.Fatal(err)
	}
	if head := r.reg.Count(registry.HeadManifest); head != 2 {
		t.Errorf("HEAD %d after the lifetime", head)
	}
	if r.counters.TagsFresh.Load() != 2 || r.counters.TagsChecked.Load() != 2 {
		t.Errorf("fresh %d, checked %d", r.counters.TagsFresh.Load(), r.counters.TagsChecked.Load())
	}
}

func TestASlowUpstreamGetsTheStaleTagInTime(t *testing.T) {
	r := newRig(t, registry.Options{})
	d := r.reg.PutManifest("library/redis", "8", index, []byte(`{"r":8}`))
	if _, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "8", nil); err != nil {
		t.Fatal(err)
	}
	r.manifests.AnswerWithin = 200 * time.Millisecond
	release := r.reg.Stall()
	defer release()
	start := time.Now()
	got, err := r.manifests.Get(t.Context(), "docker.io", "library/redis", "8", nil)
	if err != nil || got.Digest != d {
		t.Fatalf("%+v %v", got, err)
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Errorf("waited %s", waited)
	}
	if r.counters.TagsStale.Load() != 1 {
		t.Errorf("stale %d", r.counters.TagsStale.Load())
	}
}
