// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package oci

import (
	"fmt"
	"strings"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Platform is what an image variant runs on: an OS, a CPU architecture and, for some, a variant
// (arm64's v8, arm's v7).
type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
}

// ParsePlatform reads os/arch[/variant], such as linux/amd64 or linux/arm/v7.
func ParsePlatform(s string) (Platform, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return Platform{}, fmt.Errorf("platform %q: want os/arch[/variant]: %w", s, errkind.ErrInvalid)
	}
	p := Platform{OS: parts[0], Architecture: parts[1]}
	if len(parts) == 3 {
		p.Variant = parts[2]
	}
	return p, nil
}

// Matches reports whether p is the platform want asks for. A want without a variant takes any
// variant of its architecture.
func (p Platform) Matches(want Platform) bool {
	return p.OS == want.OS && p.Architecture == want.Architecture && (want.Variant == "" || p.Variant == want.Variant)
}

// String spells the platform as os/arch[/variant].
func (p Platform) String() string {
	if p.Variant == "" {
		return p.OS + "/" + p.Architecture
	}
	return p.OS + "/" + p.Architecture + "/" + p.Variant
}
