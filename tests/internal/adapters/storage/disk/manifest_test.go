// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package disk_test

// Verifies:
//   - A manifest comes back with its media type and bytes
//   - One never stored is not found

import (
	"testing"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestAManifestComesBackWithItsMediaType(t *testing.T) {
	s, _ := open(t)
	body := []byte(`{"schemaVersion":2}`)
	d := digest.Of(body)
	const mt = "application/vnd.oci.image.index.v1+json"
	if err := s.PutManifest(d, mt, body); err != nil {
		t.Fatal(err)
	}
	gotType, gotBody, err := s.Manifest(d)
	if err != nil || gotType != mt || string(gotBody) != string(body) {
		t.Fatalf("%q %q %v", gotType, gotBody, err)
	}
	if _, _, err := s.Manifest(digest.Of([]byte("other"))); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing: %v", err)
	}
}
