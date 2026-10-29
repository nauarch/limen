// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package image_test

// Verifies:
//   - Docker Hub's aliases, an empty name and any casing fold into docker.io; others keep their name
//   - Docker Hub's API answers on registry-1.docker.io, other registries on their own name

import (
	"testing"

	"github.com/nauarch/limen/internal/kernel/image"
)

func TestDockerHubsNamesFoldIntoOne(t *testing.T) {
	for in, want := range map[string]string{
		"":                        "docker.io",
		"docker.io":               "docker.io",
		"Docker.IO":               "docker.io",
		"index.docker.io":         "docker.io",
		"registry-1.docker.io":    "docker.io",
		"registry.hub.docker.com": "docker.io",
		"ghcr.io":                 "ghcr.io",
		" Quay.io ":               "quay.io",
	} {
		if got := image.Canonical(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestDockerHubsAPILivesApartFromItsName(t *testing.T) {
	for in, want := range map[string]string{"docker.io": "registry-1.docker.io", "": "registry-1.docker.io", "ghcr.io": "ghcr.io"} {
		if got := image.APIHost(in); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}
