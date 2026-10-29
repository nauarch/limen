// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package image_test

// Verifies:
//   - A reference splits into registry (Docker Hub when none is named, library/ for its official
//     images), repository, and tag (latest when none) or digest; a port is not a tag
//   - A malformed reference is invalid
//   - A reference spells itself in full and names what a manifest request asks for

import (
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/kernel/image"
)

func TestAReferenceSaysWhereItIsPulledFrom(t *testing.T) {
	d := digest.Of([]byte("m"))
	for in, want := range map[string]image.Ref{
		"nginx":                         {Registry: "docker.io", Repository: "library/nginx", Tag: "latest"},
		"vllm/vllm-openai:v0.32.0":      {Registry: "docker.io", Repository: "vllm/vllm-openai", Tag: "v0.32.0"},
		"docker.io/library/redis:8":     {Registry: "docker.io", Repository: "library/redis", Tag: "8"},
		"index.docker.io/me/app:2":      {Registry: "docker.io", Repository: "me/app", Tag: "2"},
		"ghcr.io/me/app:1.0.0":          {Registry: "ghcr.io", Repository: "me/app", Tag: "1.0.0"},
		"registry.local:5050/home/hi:1": {Registry: "registry.local:5050", Repository: "home/hi", Tag: "1"},
		"localhost:5000/app":            {Registry: "localhost:5000", Repository: "app", Tag: "latest"},
		"ghcr.io/me/app@" + d.String():  {Registry: "ghcr.io", Repository: "me/app", Digest: d},
		"me/app:1.0@" + d.String():      {Registry: "docker.io", Repository: "me/app", Digest: d},
	} {
		if got, err := image.Parse(in); err != nil || got != want {
			t.Errorf("%s = %+v, %v; want %+v", in, got, err, want)
		}
	}
}

func TestAMalformedReferenceIsInvalid(t *testing.T) {
	for _, bad := range []string{"", "a b", "Nginx", "nginx:.bad", "me/app@sha256:abc", "ghcr.io/", strings.Repeat("a/", 200) + "a"} {
		if _, err := image.Parse(bad); errkind.Of(err) != errkind.Invalid {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestAReferenceSpellsItself(t *testing.T) {
	r, _ := image.Parse("nginx")
	if r.String() != "docker.io/library/nginx:latest" || r.Reference() != "latest" {
		t.Errorf("%s %s", r, r.Reference())
	}
	d := digest.Of([]byte("m"))
	p, _ := image.Parse("ghcr.io/me/app@" + d.String())
	if p.String() != "ghcr.io/me/app@"+d.String() || p.Reference() != d.String() {
		t.Errorf("%s %s", p, p.Reference())
	}
}
