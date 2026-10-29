// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cache_test

// Verifies:
//   - The cache needs a store and an upstream
//   - It serves the registries it was given, by any of their names, and refuses others
//   - A malformed repository is refused before anything is asked

import (
	"context"
	"io"
	"testing"

	"github.com/nauarch/limen/internal/adapters/storage/disk"
	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/features/cache"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// noUpstream fails every request, so a test sees whether one was made.
type noUpstream struct{ t *testing.T }

// Resolve fails the test.
func (u noUpstream) Resolve(context.Context, string, string, string, []string) (digest.Digest, string, error) {
	u.t.Error("upstream asked")
	return "", "", errkind.ErrUnreachable
}

// Manifest fails the test.
func (u noUpstream) Manifest(context.Context, string, string, string, []string) (pull.Manifest, error) {
	u.t.Error("upstream asked")
	return pull.Manifest{}, errkind.ErrUnreachable
}

// Blob fails the test.
func (u noUpstream) Blob(context.Context, string, string, digest.Digest) (io.ReadCloser, int64, error) {
	u.t.Error("upstream asked")
	return nil, 0, errkind.ErrUnreachable
}

// compose is a cache serving docker.io and ghcr.io that must not reach upstream.
func compose(t *testing.T) *cache.Cache {
	t.Helper()
	store, err := disk.Open(disk.Options{Root: t.TempDir(), Init: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := cache.Compose(cache.Options{Store: store, Upstream: noUpstream{t}, Registries: []string{"docker.io", "GHCR.io"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheCacheNeedsAStoreAndAnUpstream(t *testing.T) {
	if _, err := cache.Compose(cache.Options{}); errkind.Of(err) != errkind.Invalid {
		t.Fatalf("got %v", err)
	}
}

func TestOnlyListedRegistriesAreServed(t *testing.T) {
	c := compose(t)
	if got := c.Registries(); len(got) != 2 || got[0] != "docker.io" || got[1] != "ghcr.io" {
		t.Errorf("listed %v", got)
	}
	for name, want := range map[string]bool{"docker.io": true, "registry-1.docker.io": true, "": true, "ghcr.io": true, "quay.io": false} {
		if got := c.Serves(name); got != want {
			t.Errorf("%q: %v", name, got)
		}
	}
	if _, err := c.Manifest(t.Context(), "quay.io", "a/b", "latest", nil); errkind.Of(err) != errkind.Forbidden {
		t.Errorf("unlisted registry: %v", err)
	}
}

func TestAMalformedRepositoryIsRefused(t *testing.T) {
	c := compose(t)
	if _, err := c.Blob(t.Context(), "docker.io", "../../etc", digest.Of(nil)); errkind.Of(err) != errkind.Invalid {
		t.Errorf("got %v", err)
	}
}
