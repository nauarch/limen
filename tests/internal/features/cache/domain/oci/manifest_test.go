// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package oci_test

// Verifies:
//   - An index lists its variants with their platforms; one without a platform has an empty one
//   - An image manifest lists its config, then its layers
//   - Anything else is invalid

import (
	"testing"

	"github.com/nauarch/limen/internal/features/cache/domain/oci"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// d is a digest for a name.
func d(name string) digest.Digest { return digest.Of([]byte(name)) }

func TestAnIndexListsItsVariants(t *testing.T) {
	body := []byte(`{"schemaVersion":2,"manifests":[
		{"digest":"` + d("amd").String() + `","platform":{"os":"linux","architecture":"amd64"}},
		{"digest":"` + d("arm").String() + `","platform":{"os":"linux","architecture":"arm64","variant":"v8"}},
		{"digest":"` + d("att").String() + `"}]}`)
	got, err := oci.Parse(body)
	if err != nil || len(got.Children) != 3 || len(got.Blobs) != 0 {
		t.Fatalf("%+v %v", got, err)
	}
	if got.Children[1].Digest != d("arm") || got.Children[1].Platform.String() != "linux/arm64/v8" || got.Children[2].Platform != (oci.Platform{}) {
		t.Errorf("%+v", got.Children)
	}
}

func TestAnImageManifestListsItsConfigThenLayers(t *testing.T) {
	body := []byte(`{"schemaVersion":2,"config":{"digest":"` + d("cfg").String() + `","size":10},
		"layers":[{"digest":"` + d("l1").String() + `","size":100},{"digest":"` + d("l2").String() + `","size":200}]}`)
	got, err := oci.Parse(body)
	if err != nil || len(got.Blobs) != 3 || got.Blobs[0].Digest != d("cfg") || got.Blobs[2].Size != 200 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestAnythingElseIsInvalid(t *testing.T) {
	for _, bad := range []string{`not json`, `{"schemaVersion":1,"fsLayers":[]}`, `{"manifests":[{"digest":"sha256:abc"}]}`} {
		if _, err := oci.Parse([]byte(bad)); errkind.Of(err) != errkind.Invalid {
			t.Errorf("%s: %v", bad, err)
		}
	}
}
