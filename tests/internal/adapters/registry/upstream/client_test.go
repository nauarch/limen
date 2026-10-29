// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream_test

// Verifies:
//   - A registry that asks for a token gets an anonymous one, fetched once and reused while it lives
//   - A token past its lifetime is fetched again
//   - A registry that cannot be reached is unreachable

import (
	"context"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/adapters/registry/upstream"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	"github.com/nauarch/limen/tests/kit/registry"
)

// client reaches reg as docker.io, on clk.
func client(reg *registry.Registry, clk *kitclock.Fixed) *upstream.Client {
	return upstream.New(upstream.Options{Endpoints: map[string]string{"docker.io": reg.URL()}, Clock: clk})
}

func TestATokenIsFetchedOnceAndReused(t *testing.T) {
	reg := registry.New(registry.Options{RequireToken: true})
	defer reg.Close()
	reg.PutManifest("library/nginx", "latest", "application/vnd.oci.image.index.v1+json", []byte(`{"n":1}`))
	clk := kitclock.At(time.Date(2026, 10, 29, 5, 0, 0, 0, time.UTC))
	c := client(reg, clk)
	ctx := context.Background()
	for range 3 {
		if _, err := c.Manifest(ctx, "docker.io", "library/nginx", "latest", nil); err != nil {
			t.Fatal(err)
		}
	}
	if n := reg.Count(registry.Token); n != 1 {
		t.Fatalf("%d tokens fetched for 3 requests", n)
	}
	clk.Advance(10 * time.Minute)
	if _, err := c.Manifest(ctx, "docker.io", "library/nginx", "latest", nil); err != nil {
		t.Fatal(err)
	}
	if n := reg.Count(registry.Token); n != 2 {
		t.Errorf("%d tokens after the first expired", n)
	}
}

func TestAnUnreachableRegistryIsUnreachable(t *testing.T) {
	reg := registry.New(registry.Options{})
	c := client(reg, kitclock.At(time.Now()))
	reg.Close()
	_, err := c.Manifest(context.Background(), "docker.io", "library/nginx", "latest", nil)
	if errkind.Of(err) != errkind.Unreachable {
		t.Fatalf("got %v", err)
	}
}
