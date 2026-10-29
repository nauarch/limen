// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package scripts_test

// Verifies:
//   - A script runs in its project's folder
//   - A failing script is an Upstream error carrying the end of what it printed

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/adapters/host/scripts"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// fakeEnv makes this test binary act as a script: it notes the folder it ran in to the file the
// variable names, or, when that is "fail", prints and fails.
const fakeEnv = "LIMEN_FAKE_SCRIPT"

func TestMain(m *testing.M) {
	switch out := os.Getenv(fakeEnv); out {
	case "":
		os.Exit(m.Run())
	case "fail":
		fmt.Println(strings.Repeat("noise ", 1000))
		fmt.Fprintln(os.Stderr, "the resolver did not stop")
		os.Exit(3)
	default:
		dir, _ := os.Getwd()
		_ = os.WriteFile(out, []byte(dir), 0o600)
		os.Exit(0)
	}
}

func TestAScriptRunsInItsFolder(t *testing.T) {
	out, dir := t.TempDir()+"/ran", t.TempDir()
	t.Setenv(fakeEnv, out)
	if err := (scripts.Exec{}).Run(context.Background(), os.Args[0], dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); !sameDir(string(got), dir) {
		t.Errorf("ran in %s, want %s", got, dir)
	}
}

func TestAFailingScriptSaysWhy(t *testing.T) {
	t.Setenv(fakeEnv, "fail")
	err := (scripts.Exec{}).Run(context.Background(), os.Args[0], t.TempDir())
	if errkind.Of(err) != errkind.Upstream || !strings.Contains(err.Error(), "the resolver did not stop") || len(err.Error()) > 3<<10 {
		t.Errorf("%d bytes: %.200v", len(err.Error()), err)
	}
}

// sameDir compares folders as the system spells them (short and long names alike on Windows).
func sameDir(a, b string) bool {
	ia, erra := os.Stat(a)
	ib, errb := os.Stat(b)
	return erra == nil && errb == nil && os.SameFile(ia, ib)
}
