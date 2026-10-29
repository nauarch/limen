// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package verify checks that a host's pulls reach limen: it pulls a small image and compares
// what limen counted from the host before and after.
package verify

import (
	"context"
	"fmt"
	"slices"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
)

// Image is the image pulled: tiny, on Docker Hub, built for every common platform.
const Image = "docker.io/library/hello-world:latest"

// Verifier pulls through the host's engine and asks the limen.
type Verifier struct {
	Engine host.Engine
	Limen  func(url string) host.Limen
}

// Verify pulls Image and reports how many of the host's requests reached the limen meanwhile.
// Even an image the host has makes the engine ask the registry for its tag, so the pull always
// sends at least one request. The image is removed again when the host did not have it.
func (v *Verifier) Verify(ctx context.Context, limen string) (hostplan.Verification, error) {
	l := v.Limen(limen)
	before, err := l.Me(ctx)
	if err != nil {
		return hostplan.Verification{}, fmt.Errorf("asking limen at %s: %w", limen, err)
	}
	had, err := v.has(ctx)
	if err != nil {
		return hostplan.Verification{}, err
	}
	if err := v.Engine.Pull(ctx, Image); err != nil {
		return hostplan.Verification{}, fmt.Errorf("pulling %s: %w", Image, err)
	}
	after, err := l.Me(ctx)
	if err != nil {
		return hostplan.Verification{}, fmt.Errorf("asking limen at %s: %w", limen, err)
	}
	out := hostplan.Verification{Image: Image, Requests: after.Requests - before.Requests}
	if !had {
		if err := v.Engine.RemoveImage(ctx, Image); err != nil {
			return out, fmt.Errorf("removing %s again: %w", Image, err)
		}
		out.Removed = true
	}
	return out, nil
}

// has reports whether the host has Image, by any of the names Docker gives it.
func (v *Verifier) has(ctx context.Context) (bool, error) {
	images, err := v.Engine.Images(ctx)
	if err != nil {
		return false, err
	}
	names := []string{Image, "hello-world:latest", "library/hello-world:latest"}
	return slices.ContainsFunc(images, func(im host.Image) bool {
		return slices.ContainsFunc(im.Tags, func(t string) bool { return slices.Contains(names, t) })
	}), nil
}
