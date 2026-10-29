// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ctl_test

// Verifies:
//   - limenctl composes against the real host adapters and answers help without touching Docker
//   - A command line it cannot run exits 2; version names the build

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/ctl"
)

func TestLimenctlComposesAndAnswers(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := ctl.Run(context.Background(), "v1.2.3", []string{"help"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "limenctl") {
		t.Errorf("%d %s %s", code, out.String(), errOut.String())
	}
	if code := ctl.Run(context.Background(), "v1.2.3", []string{"configure"}, &out, &errOut); code != 2 {
		t.Errorf("configure without --limen: %d", code)
	}
	out.Reset()
	if code := ctl.Run(context.Background(), "v1.2.3", []string{"version"}, &out, &errOut); code != 0 || out.String() != "limenctl v1.2.3 (https://github.com/nauarch/limen, Apache-2.0)\n" {
		t.Errorf("version: %d %q", code, out.String())
	}
}
