// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package desktop_test

// Verifies:
//   - Restarting runs docker desktop restart, whatever the unit
//   - A failing docker desktop is an Upstream error carrying what it printed

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/adapters/host/desktop"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// fakeEnv makes this test binary act as docker: it fails when the variable says "fail", and
// otherwise writes its arguments to the file the variable names.
const fakeEnv = "LIMEN_FAKE_DOCKER_DESKTOP"

func TestMain(m *testing.M) {
	switch out := os.Getenv(fakeEnv); out {
	case "":
		os.Exit(m.Run())
	case "fail":
		fmt.Println("Docker Desktop is not running")
		os.Exit(1)
	default:
		_ = os.WriteFile(out, []byte(strings.Join(os.Args[1:], " ")), 0o600)
		os.Exit(0)
	}
}

func TestRestartingRunsDockerDesktopRestart(t *testing.T) {
	out := t.TempDir() + "/args"
	t.Setenv(fakeEnv, out)
	if err := (desktop.Desktop{Command: os.Args[0]}).Restart(context.Background(), "docker"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "desktop restart" {
		t.Errorf("ran %q", got)
	}
}

func TestAFailingRestartIsAnError(t *testing.T) {
	t.Setenv(fakeEnv, "fail")
	err := (desktop.Desktop{Command: os.Args[0]}).Restart(context.Background(), "docker")
	if errkind.Of(err) != errkind.Upstream || !strings.Contains(err.Error(), "not running") {
		t.Errorf("%v", err)
	}
}
