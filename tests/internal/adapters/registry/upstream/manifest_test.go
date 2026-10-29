// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream_test

// Verifies:
//   - Resolve tells a tag's digest and media type with a HEAD alone
//   - A manifest comes with its media type and digest
//   - A missing tag is not found

import (
	"context"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	"github.com/nauarch/limen/tests/kit/registry"
)

func TestResolveAsksWithAHeadAlone(t *testing.T) {
	reg := registry.New(registry.Options{})
	defer reg.Close()
	const mt = "application/vnd.oci.image.index.v1+json"
	want := reg.PutManifest("vllm/vllm-openai", "v0.32.0", mt, []byte(`{"v":32}`))
	c := client(reg, kitclock.At(time.Now()))
	d, gotType, err := c.Resolve(context.Background(), "docker.io", "vllm/vllm-openai", "v0.32.0", []string{mt})
	if err != nil || d != want || gotType != mt {
		t.Fatalf("%s %s %v", d, gotType, err)
	}
	if reg.Count(registry.GetManifest) != 0 || reg.Count(registry.HeadManifest) != 1 {
		t.Errorf("GET %d, HEAD %d", reg.Count(registry.GetManifest), reg.Count(registry.HeadManifest))
	}
}

func TestAManifestComesWithItsTypeAndDigest(t *testing.T) {
	reg := registry.New(registry.Options{})
	defer reg.Close()
	want := reg.PutManifest("library/redis", "8", "application/vnd.oci.image.manifest.v1+json", []byte(`{"r":8}`))
	c := client(reg, kitclock.At(time.Now()))
	got, err := c.Manifest(context.Background(), "docker.io", "library/redis", "8", nil)
	if err != nil || got.Digest != want || got.MediaType != "application/vnd.oci.image.manifest.v1+json" || string(got.Body) != `{"r":8}` {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := c.Manifest(context.Background(), "docker.io", "library/redis", "9", nil); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing tag: %v", err)
	}
	if _, _, err := c.Resolve(context.Background(), "docker.io", "library/redis", "9", nil); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing tag, HEAD: %v", err)
	}
}
