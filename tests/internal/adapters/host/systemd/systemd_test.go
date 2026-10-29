// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package systemd_test

// Verifies:
//   - Restarting runs systemctl restart <unit>
//   - A failing systemctl is an Upstream error carrying what it printed
//   - A systemctl that never returns is ended by the timeout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/adapters/host/systemd"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// fakeEnv makes this test binary act as systemctl: it hangs for the unit "hang", fails for
// "broken", and otherwise writes its arguments to the file the variable names.
const fakeEnv = "LIMEN_FAKE_SYSTEMCTL"

func TestMain(m *testing.M) {
	if out := os.Getenv(fakeEnv); out != "" {
		if os.Args[len(os.Args)-1] == "hang" {
			time.Sleep(time.Minute)
		}
		if os.Args[len(os.Args)-1] == "broken" {
			fmt.Fprintln(os.Stderr, "Unit broken.service not found.")
			os.Exit(5)
		}
		_ = os.WriteFile(out, []byte(strings.Join(os.Args[1:], " ")), 0o600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fake is a Systemd running this binary as systemctl, and the file it notes its arguments in.
func fake(t *testing.T) (systemd.Systemd, string) {
	out := t.TempDir() + "/args"
	t.Setenv(fakeEnv, out)
	return systemd.Systemd{Command: os.Args[0]}, out
}

func TestRestartingRunsSystemctl(t *testing.T) {
	s, out := fake(t)
	if err := s.Restart(context.Background(), "docker"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); string(got) != "restart docker" {
		t.Errorf("ran %q", got)
	}
}

func TestAFailingSystemctlIsAnError(t *testing.T) {
	s, _ := fake(t)
	err := s.Restart(context.Background(), "broken")
	if errkind.Of(err) != errkind.Upstream || !strings.Contains(err.Error(), "Unit broken.service not found.") {
		t.Errorf("%v", err)
	}
}

func TestAHangingSystemctlTimesOut(t *testing.T) {
	s, _ := fake(t)
	s.Timeout = 200 * time.Millisecond
	start := time.Now()
	err := s.Restart(context.Background(), "hang")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 30*time.Second {
		t.Errorf("%v after %s", err, time.Since(start))
	}
}
