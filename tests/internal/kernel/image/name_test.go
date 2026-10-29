// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package image_test

// Verifies:
//   - Repository names follow the registry grammar: lower case, separators inside components,
//     no empty or dot-dot components, no longer than 255
//   - Tags follow theirs: up to 128 characters, not starting with '.' or '-'

import (
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/kernel/image"
)

func TestRepositoryNamesFollowTheGrammar(t *testing.T) {
	for _, ok := range []string{"library/nginx", "vllm/vllm-openai", "a/b/c", "linuxserver/code-server", "a__b", "x.y/z"} {
		if !image.ValidRepository(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "Library/nginx", "a//b", "../etc", "a/../b", "/a", "a/", "-a", "a b", strings.Repeat("a", 256)} {
		if image.ValidRepository(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestTagsFollowTheGrammar(t *testing.T) {
	for _, ok := range []string{"latest", "v0.32.0", "1.27-alpine", "_x", strings.Repeat("a", 128)} {
		if !image.ValidTag(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", ".x", "-x", "a/b", "a:b", strings.Repeat("a", 129)} {
		if image.ValidTag(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
