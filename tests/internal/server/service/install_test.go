// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package service_test

// Verifies:
//   - On a fresh systemd host install copies limen in, makes its user and a marked store it owns,
//     writes the unit, enables and starts it, and waits for /healthz; nothing happens before Do
//   - Installed again with the same arguments, only the restart and the wait are left
//   - A folder holding files without the marker, a unit not its own, or a host without systemd
//     is refused
//   - Uninstall stops and removes the service and binary, keeping the store

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/server/service"
)

// host is a fresh systemd host under a temporary root, and the commands run on it.
type host struct {
	in  service.Installer
	ran *[]string
}

func newHost(t *testing.T) host {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "run", "systemd", "system"), 0o755); err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(t.TempDir(), "limen")
	if err := os.WriteFile(self, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var ran []string
	users := map[string]bool{}
	return host{ran: &ran, in: service.Installer{
		Root: root, Self: self,
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			line := strings.Join(append([]string{name}, args...), " ")
			ran = append(ran, line)
			switch {
			case name == "id" && !users[args[1]]:
				return nil, errkind.ErrNotFound
			case name == "useradd":
				users[args[len(args)-1]] = true
			}
			return nil, nil
		},
		Healthy: func(context.Context) error { return nil },
	}}
}

// do takes the steps.
func do(t *testing.T, steps []service.Step) {
	t.Helper()
	for _, s := range steps {
		if err := s.Do(context.Background()); err != nil {
			t.Fatalf("%s: %v", s.Say, err)
		}
	}
}

// says lists what the steps say, first lines only.
func says(steps []service.Step) []string {
	var out []string
	for _, s := range steps {
		first, _, _ := strings.Cut(s.Say, "\n")
		out = append(out, first)
	}
	return out
}

var args = []string{"--dir=/var/lib/limen", "--max-size=300G"}

func TestAFreshHostGetsLimenInstalled(t *testing.T) {
	h := newHost(t)
	steps, err := h.in.Install(context.Background(), "/var/lib/limen", args)
	if err != nil {
		t.Fatal(err)
	}
	if len(*h.ran) != 1 || (*h.ran)[0] != "id -u limen" {
		t.Errorf("planning ran %v", *h.ran)
	}
	do(t, steps)
	root := h.in.Root
	if data, _ := os.ReadFile(filepath.Join(root, "usr", "local", "bin", "limen")); string(data) != "binary" {
		t.Errorf("binary %q", data)
	}
	if _, err := os.Stat(filepath.Join(root, "var", "lib", "limen", ".limen-volume")); err != nil {
		t.Errorf("store not marked: %v", err)
	}
	unit, _ := os.ReadFile(filepath.Join(root, "etc", "systemd", "system", "limen.service"))
	if !strings.Contains(string(unit), "ExecStart=/usr/local/bin/limen --dir=/var/lib/limen --max-size=300G") {
		t.Errorf("unit\n%s", unit)
	}
	for _, want := range []string{"useradd --system", "chown -R limen:limen /var/lib/limen", "systemctl daemon-reload", "systemctl enable limen", "systemctl restart limen"} {
		if !slices.ContainsFunc(*h.ran, func(r string) bool { return strings.HasPrefix(r, want) }) {
			t.Errorf("never ran %s: %v", want, *h.ran)
		}
	}
	again, err := h.in.Install(context.Background(), "/var/lib/limen", args)
	if err != nil {
		t.Fatal(err)
	}
	if got := says(again); !slices.Equal(got, []string{"copy " + h.in.Self + " to /usr/local/bin/limen", "use /var/lib/limen, limen's store already, owned by limen", "enable limen and (re)start it", "wait until limen answers /healthz"}) {
		t.Errorf("again: %q", got)
	}
}

func TestWhatInstallRefuses(t *testing.T) {
	h := newHost(t)
	busy := filepath.Join(h.in.Root, "srv", "data")
	_ = os.MkdirAll(busy, 0o755)
	_ = os.WriteFile(filepath.Join(busy, "photo.jpg"), []byte("x"), 0o644)
	if _, err := h.in.Install(context.Background(), "/srv/data", args); errkind.Of(err) != errkind.Conflict || !strings.Contains(err.Error(), "not limen's store") {
		t.Errorf("busy folder: %v", err)
	}
	unit := filepath.Join(h.in.Root, "etc", "systemd", "system", "limen.service")
	_ = os.MkdirAll(filepath.Dir(unit), 0o755)
	_ = os.WriteFile(unit, []byte("[Unit]\n"), 0o644)
	if _, err := h.in.Install(context.Background(), "/var/lib/limen", args); errkind.Of(err) != errkind.Conflict {
		t.Errorf("foreign unit: %v", err)
	}
	if _, err := h.in.Uninstall(context.Background()); errkind.Of(err) != errkind.Conflict {
		t.Errorf("uninstalling a foreign unit: %v", err)
	}
	_ = os.RemoveAll(filepath.Join(h.in.Root, "run"))
	if _, err := h.in.Install(context.Background(), "/var/lib/limen", args); errkind.Of(err) != errkind.Invalid {
		t.Errorf("no systemd: %v", err)
	}
	if _, err := h.in.Install(context.Background(), "var/lib/limen", args); errkind.Of(err) != errkind.Invalid {
		t.Errorf("relative dir: %v", err)
	}
}

func TestUninstallKeepsTheStore(t *testing.T) {
	h := newHost(t)
	steps, _ := h.in.Install(context.Background(), "/var/lib/limen", args)
	do(t, steps)
	steps, err := h.in.Uninstall(context.Background())
	if err != nil || len(steps) != 2 {
		t.Fatalf("%v %v", says(steps), err)
	}
	do(t, steps)
	for _, gone := range []string{"etc/systemd/system/limen.service", "usr/local/bin/limen"} {
		if _, err := os.Stat(filepath.Join(h.in.Root, gone)); !os.IsNotExist(err) {
			t.Errorf("%s left: %v", gone, err)
		}
	}
	if _, err := os.Stat(filepath.Join(h.in.Root, "var", "lib", "limen", ".limen-volume")); err != nil {
		t.Errorf("store removed: %v", err)
	}
	if !slices.Contains(*h.ran, "systemctl disable --now limen") {
		t.Errorf("%v", *h.ran)
	}
	if again, err := h.in.Uninstall(context.Background()); err != nil || again != nil {
		t.Errorf("nothing left to uninstall: %v %v", says(again), err)
	}
}
