// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"path/filepath"
	"strings"

	"github.com/nauarch/limen/internal/kernel/digest"
)

// blobPath is where a blob lives: blobs/sha256/ab/abcdef…, fanned out by its first two digits.
func (s *Dir) blobPath(d digest.Digest) string {
	h := d.Hex()
	return filepath.Join(s.root, "blobs", digest.Algorithm, h[:2], h)
}

// manifestPath is where a manifest lives; its media type sits beside it in a .type file.
func (s *Dir) manifestPath(d digest.Digest) string {
	h := d.Hex()
	return filepath.Join(s.root, "manifests", digest.Algorithm, h[:2], h)
}

// tagPath is where a tag's record lives: tags/<registry>/<repository>/<tag>.json. A registry's
// port separator becomes '+', since SMB refuses ':'. Repository and tag are checked against the
// registry grammar before they get here, so they hold no "..".
func (s *Dir) tagPath(registry, repository, tag string) string {
	return filepath.Join(s.root, "tags", strings.ReplaceAll(registry, ":", "+"), filepath.FromSlash(repository), tag+".json")
}
