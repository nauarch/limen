// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"fmt"
	"strings"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/kernel/image"
)

// Reference is what a manifest request names: a digest, which never changes and is served from
// the store whenever it is there, or a tag, which upstream is asked about.
type Reference struct {
	Digest digest.Digest
	Tag    string
}

// ParseReference reads a manifest reference: a digest when it has the algorithm's prefix, a tag
// otherwise.
func ParseReference(s string) (Reference, error) {
	if strings.Contains(s, ":") {
		d, err := digest.Parse(s)
		if err != nil {
			return Reference{}, err
		}
		return Reference{Digest: d}, nil
	}
	if !image.ValidTag(s) {
		return Reference{}, fmt.Errorf("tag %q: %w", s, errkind.ErrInvalid)
	}
	return Reference{Tag: s}, nil
}
