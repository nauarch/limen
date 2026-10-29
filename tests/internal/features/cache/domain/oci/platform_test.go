// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package oci_test

// Verifies:
//   - A platform reads as os/arch[/variant]; anything else is invalid
//   - A wanted platform without a variant matches every variant of its architecture

import (
	"testing"

	"github.com/nauarch/limen/internal/features/cache/domain/oci"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestAPlatformReadsAsOSArchVariant(t *testing.T) {
	p, err := oci.ParsePlatform("Linux/ARM/v7")
	if err != nil || p != (oci.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}) {
		t.Fatalf("%+v %v", p, err)
	}
	for _, bad := range []string{"", "linux", "linux/", "/amd64", "a/b/c/d"} {
		if _, err := oci.ParsePlatform(bad); errkind.Of(err) != errkind.Invalid {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestAWantedPlatformWithoutVariantTakesAny(t *testing.T) {
	arm64v8 := oci.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"}
	for want, ok := range map[string]bool{"linux/arm64": true, "linux/arm64/v8": true, "linux/arm64/v9": false, "linux/amd64": false} {
		w, _ := oci.ParsePlatform(want)
		if got := arm64v8.Matches(w); got != ok {
			t.Errorf("%s: %v", want, got)
		}
	}
}
