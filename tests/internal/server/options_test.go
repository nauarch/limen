// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package server_test

// Verifies:
//   - Without a store folder limen refuses to run
//   - Left empty, the address is :5000 and the registries are the public defaults

import (
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/server"
)

func TestDefaultsFillWhatIsLeftEmpty(t *testing.T) {
	if _, err := server.Defaults(server.Options{}); errkind.Of(err) != errkind.Invalid {
		t.Errorf("no folder: %v", err)
	}
	o, err := server.Defaults(server.Options{Dir: "x"})
	if err != nil || o.Addr != ":5000" || !slices.Contains(o.Registries, "docker.io") || !slices.Contains(o.Registries, "ghcr.io") {
		t.Fatalf("%+v %v", o, err)
	}
}
