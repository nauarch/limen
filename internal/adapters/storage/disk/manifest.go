// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// typeSuffix names the file beside a manifest that holds its media type.
const typeSuffix = ".type"

// Manifest reads a manifest and its media type.
func (s *Dir) Manifest(d digest.Digest) (string, []byte, error) {
	path := s.manifestPath(d)
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, fmt.Errorf("manifest %s: %w", d, errkind.ErrNotFound)
	}
	if err != nil {
		return "", nil, fmt.Errorf("store: %w", err)
	}
	mediaType, err := os.ReadFile(path + typeSuffix)
	if err != nil {
		return "", nil, fmt.Errorf("manifest %s has no media type: %w", d, errkind.ErrNotFound)
	}
	return string(mediaType), body, nil
}

// PutManifest keeps a manifest: its media type first, so a manifest that is there always has one.
func (s *Dir) PutManifest(d digest.Digest, mediaType string, body []byte) error {
	path := s.manifestPath(d)
	if err := s.writeFile(path+typeSuffix, []byte(mediaType)); err != nil {
		return err
	}
	return s.writeFile(path, body)
}
