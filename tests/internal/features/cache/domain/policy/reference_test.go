// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package policy_test

// Verifies:
//   - A reference with the algorithm prefix is a digest, anything else a tag
//   - A malformed digest or tag is invalid

import (
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/features/cache/domain/policy"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestAReferenceIsADigestOrATag(t *testing.T) {
	d := digest.Of([]byte("m"))
	if got, err := policy.ParseReference(d.String()); err != nil || got.Digest != d || got.Tag != "" {
		t.Errorf("digest: %+v, %v", got, err)
	}
	if got, err := policy.ParseReference("v0.32.0"); err != nil || got.Tag != "v0.32.0" || got.Digest != "" {
		t.Errorf("tag: %+v, %v", got, err)
	}
}

func TestAMalformedReferenceIsInvalid(t *testing.T) {
	for _, bad := range []string{"sha256:abc", "md5:" + strings.Repeat("a", 32), ".hidden", ""} {
		if _, err := policy.ParseReference(bad); errkind.Of(err) != errkind.Invalid {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
