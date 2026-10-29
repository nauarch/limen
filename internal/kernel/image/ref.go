// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package image

import (
	"fmt"
	"strings"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Ref is an image reference split into where it is pulled from: the registry (Hub when the
// reference names none), the repository inside it (library/nginx on Docker Hub for nginx), and the
// tag or the digest it names. A reference with both keeps the digest; one with neither means
// latest.
type Ref struct {
	Registry   string
	Repository string
	Tag        string
	Digest     digest.Digest
}

// Parse splits an image reference such as nginx, ghcr.io/me/app:1.0 or me/app@sha256:….
func Parse(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	bad := func(why string) (Ref, error) {
		return Ref{}, fmt.Errorf("image %q: %s: %w", s, why, errkind.ErrInvalid)
	}
	if s == "" || strings.ContainsAny(s, " \t") {
		return bad("empty or holds spaces")
	}
	rest := s
	ref := Ref{Registry: Hub}
	if first, after, ok := strings.Cut(s, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		ref.Registry, rest = Canonical(first), after
	}
	rest, pinned, hasDigest := strings.Cut(rest, "@")
	if hasDigest {
		d, err := digest.Parse(pinned)
		if err != nil {
			return Ref{}, err
		}
		ref.Digest = d
	}
	if c := strings.LastIndex(rest, ":"); c > strings.LastIndex(rest, "/") {
		ref.Tag, rest = rest[c+1:], rest[:c]
		if !ValidTag(ref.Tag) {
			return bad("malformed tag")
		}
	}
	if hasDigest {
		ref.Tag = ""
	} else if ref.Tag == "" {
		ref.Tag = "latest"
	}
	if ref.Registry == Hub && !strings.Contains(rest, "/") {
		rest = "library/" + rest
	}
	if !ValidRepository(rest) {
		return bad("malformed repository")
	}
	ref.Repository = rest
	return ref, nil
}

// Reference is what a manifest request names: the digest when pinned, the tag otherwise.
func (r Ref) Reference() string {
	if r.Digest != "" {
		return r.Digest.String()
	}
	return r.Tag
}

// String spells the reference in full.
func (r Ref) String() string {
	if r.Digest != "" {
		return r.Registry + "/" + r.Repository + "@" + r.Digest.String()
	}
	return r.Registry + "/" + r.Repository + ":" + r.Tag
}
