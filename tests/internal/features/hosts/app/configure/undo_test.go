// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package configure_test

// Verifies:
//   - Undoing removes limenctl's hosts.toml files (and the folders it made), takes the limen they
//     name out of daemon.json, backs everything up, and restarts Docker for daemon.json
//   - A file the owner had before --force replaced it comes back from its backup
//   - Files limenctl did not write are left alone; a host it never configured needs nothing

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
)

// configured is the rig after limenctl configure, the owner's own ghcr.io file replaced by force;
// quay.io, which this limen does not serve, keeps the owner's file.
func configured(t *testing.T) *rig {
	r := newRig(map[string]string{
		layout.Linux().DaemonJSON:           `{"log-driver":"local"}`,
		layout.Linux().HostsToml("ghcr.io"): "server = \"https://ghcr.io\"\n# the owner's\n",
		layout.Linux().HostsToml("quay.io"): "server = \"https://quay.io\"\n",
	})
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.c.Apply(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	r.c.Clock = kitclock.At(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	r.engine.AfterRestart = &host.Info{}
	r.services.Restarted = nil
	return r
}

func TestUndoingTakesLimenctlsConfigurationOff(t *testing.T) {
	r := configured(t)
	undo, err := r.c.PlanUndo(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !undo.Restart || len(undo.Skipped) != 1 || !strings.Contains(undo.Skipped[0], "quay.io") {
		t.Errorf("restart %v, skipped %v", undo.Restart, undo.Skipped)
	}
	if err := r.c.Apply(context.Background(), undo); err != nil {
		t.Fatal(err)
	}
	if _, still := r.files.Data[layout.Linux().HostsToml("docker.io")]; still {
		t.Error("docker.io's hosts.toml is still there")
	}
	if !slices.Contains(r.files.Removed, "/etc/docker/certs.d/docker.io") {
		t.Errorf("its folder stayed: %v", r.files.Removed)
	}
	if got := string(r.files.Data[layout.Linux().HostsToml("ghcr.io")]); got != "server = \"https://ghcr.io\"\n# the owner's\n" {
		t.Errorf("ghcr.io not given back: %q", got)
	}
	if got := string(r.files.Data[layout.Linux().DaemonJSON]); strings.Contains(got, limen) || !strings.Contains(got, "local") {
		t.Errorf("daemon.json %s", got)
	}
	if !slices.Equal(r.services.Restarted, []string{"docker"}) {
		t.Errorf("restarted %v", r.services.Restarted)
	}
	if again, _ := r.c.PlanUndo(context.Background(), ""); len(again.Writes) != 0 {
		t.Errorf("a second undo: %+v", again.Writes)
	}
}

func TestANeverConfiguredHostNeedsNoUndo(t *testing.T) {
	r := newRig(nil)
	if undo, err := r.c.PlanUndo(context.Background(), limen); err != nil || len(undo.Writes) != 0 || undo.Restart {
		t.Errorf("%+v %v", undo, err)
	}
}
