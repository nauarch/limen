// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/nauarch/limen/internal/contracts/storage"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Tag reads a tag's record.
func (s *Dir) Tag(registry, repository, tag string) (storage.TagRecord, error) {
	data, err := os.ReadFile(s.tagPath(registry, repository, tag))
	if errors.Is(err, fs.ErrNotExist) {
		return storage.TagRecord{}, fmt.Errorf("tag %s/%s:%s: %w", registry, repository, tag, errkind.ErrNotFound)
	}
	if err != nil {
		return storage.TagRecord{}, fmt.Errorf("store: %w", err)
	}
	var r storage.TagRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return storage.TagRecord{}, fmt.Errorf("store: tag %s/%s:%s: %w", registry, repository, tag, err)
	}
	return r, nil
}

// PutTag writes a tag's record.
func (s *Dir) PutTag(registry, repository, tag string, r storage.TagRecord) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return s.writeFile(s.tagPath(registry, repository, tag), data)
}
