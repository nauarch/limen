// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package disk_test

// Verifies:
//   - A tag's record comes back as written, also for a registry with a port
//   - A tag never recorded is not found

import (
	"testing"
	"time"

	"github.com/nauarch/limen/internal/contracts/storage"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestATagsRecordComesBack(t *testing.T) {
	s, _ := open(t)
	want := storage.TagRecord{Digest: digest.Of([]byte("m")), MediaType: "x", Fetched: time.Date(2026, 10, 29, 5, 0, 0, 0, time.UTC)}
	for _, registry := range []string{"docker.io", "registry.local:5000"} {
		if err := s.PutTag(registry, "vllm/vllm-openai", "v0.32.0", want); err != nil {
			t.Fatal(err)
		}
		got, err := s.Tag(registry, "vllm/vllm-openai", "v0.32.0")
		if err != nil || got.Digest != want.Digest || got.MediaType != want.MediaType || !got.Fetched.Equal(want.Fetched) {
			t.Errorf("%s: %+v %v", registry, got, err)
		}
	}
	if _, err := s.Tag("docker.io", "vllm/vllm-openai", "v0.31.0"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing: %v", err)
	}
}
