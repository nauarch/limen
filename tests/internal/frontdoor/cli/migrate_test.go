// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli_test

// Verifies:
//   - migrate prints its steps in order (prune, prefetch, stop last first, files, restart, start)
//     and its notes, and takes them only with --yes, printing each as it goes
//   - A refused plan prints why and exits 1 without moving
//   - rollback and cleanup-old-store are dry runs without --yes; cleanup says what it frees

import (
	"context"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/frontdoor/cli"
)

// migrator answers scripted plans, recording what it was asked and made.
type migrator struct {
	plan    hostplan.Migration
	cleanup hostplan.Cleanup
	asked   hostplan.MigrateRequest
	force   bool
	made    []string
}

func (m *migrator) PlanMigration(_ context.Context, r hostplan.MigrateRequest) (hostplan.Migration, error) {
	m.asked = r
	return m.plan, nil
}

func (m *migrator) Migrate(_ context.Context, _ hostplan.Migration, progress func(string)) error {
	progress("stopping web")
	m.made = append(m.made, "migrate")
	return nil
}

func (m *migrator) PlanRollback(context.Context) (hostplan.Migration, error) { return m.plan, nil }

func (m *migrator) Rollback(context.Context, hostplan.Migration, func(string)) error {
	m.made = append(m.made, "rollback")
	return nil
}

func (m *migrator) PlanCleanup(_ context.Context, force bool) (hostplan.Cleanup, error) {
	m.force = force
	return m.cleanup, nil
}

func (m *migrator) Cleanup(context.Context, hostplan.Cleanup, func(string)) error {
	m.made = append(m.made, "cleanup")
	return nil
}

// moving runs a command line against m, giving the exit code and what was printed.
func moving(m *migrator, args ...string) (int, string) {
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), args, cli.Options{Migrator: m, Out: &out, Err: &errOut})
	return code, out.String() + errOut.String()
}

// move is a migration of two projects, dns hooked.
func move() hostplan.Migration {
	return hostplan.Migration{
		To: hostplan.Containerd, Prune: true, PruneBytes: 3 << 20, Keep: []string{"web:previous"}, KeepBytes: 1 << 20, KeepPath: "/var/lib/limenctl/images.tar",
		Limen: "http://10.0.0.5:5000", Platform: "linux/amd64", Prefetch: []string{"nginx:alpine"},
		Projects: []hostplan.MigrateProject{{Name: "dns", Stop: "/srv/dns/stop.sh", Start: "/srv/dns/start.sh"}, {Name: "web"}},
		Writes:   []hostplan.Write{{Path: "/etc/docker/daemon.json", After: []byte("{}\n")}},
		Notes:    []string{"project old is not running"},
	}
}

func TestMigratePrintsItsStepsInOrder(t *testing.T) {
	m := &migrator{plan: move()}
	code, out := moving(m, "migrate", "--limen", "http://10.0.0.5:5000", "--prune", "--force")
	if code != 0 || len(m.made) != 0 || m.asked != (hostplan.MigrateRequest{Limen: "http://10.0.0.5:5000", Prune: true, Force: true}) {
		t.Fatalf("%d made %v asked %+v\n%s", code, m.made, m.asked, out)
	}
	order := []string{
		"1. save the images built on this host to /var/lib/limenctl/images.tar (about 1MiB", "web:previous",
		"2. remove images no container uses (up to 3MiB)", "3. limen http://10.0.0.5:5000 fetches 1 images for linux/amd64", "nginx:alpine",
		"4. stop the projects", "web: docker compose stop", "dns: /srv/dns/stop.sh",
		"5. write the files", "/etc/docker/daemon.json (new)",
		"6. restart Docker on the containerd image store", "7. load the saved images into the new store",
		"8. start the projects", "dns: /srv/dns/start.sh", "web: docker compose up --detach",
		"Worth knowing:", "project old is not running", "Dry run",
	}
	at := 0
	for _, want := range order {
		i := strings.Index(out[at:], want)
		if i < 0 {
			t.Fatalf("missing %q after byte %d in\n%s", want, at, out)
		}
		at += i + len(want)
	}
}

func TestMigrateWithYesMoves(t *testing.T) {
	m := &migrator{plan: move()}
	if code, out := moving(m, "migrate", "--yes"); code != 0 || len(m.made) != 1 || !strings.Contains(out, "-> stopping web") || !strings.Contains(out, "Done in") {
		t.Errorf("%d %v\n%s", code, m.made, out)
	}
}

func TestARefusedMoveSaysWhy(t *testing.T) {
	m := &migrator{plan: hostplan.Migration{Refusals: []string{"swarm mode is on"}, Notes: []string{"no limen given"}}}
	code, out := moving(m, "migrate", "--yes")
	if code != 1 || len(m.made) != 0 || !strings.Contains(out, "Refused:\n  - swarm mode is on") || !strings.Contains(out, "no limen given") {
		t.Errorf("%d %v\n%s", code, m.made, out)
	}
}

func TestRollbackAndCleanupAreDryRunsWithoutYes(t *testing.T) {
	m := &migrator{plan: move(), cleanup: hostplan.Cleanup{Paths: []hostplan.Removal{{Path: "/var/lib/docker/overlay2", Bytes: 2 << 30}, {Path: "/var/lib/docker/containers/old1", Bytes: 1 << 30}}}}
	if code, out := moving(m, "rollback"); code != 0 || len(m.made) != 0 || !strings.Contains(out, "Dry run") {
		t.Errorf("rollback: %d %v\n%s", code, m.made, out)
	}
	code, out := moving(m, "cleanup-old-store", "--force")
	if code != 0 || len(m.made) != 0 || !m.force || !strings.Contains(out, "/var/lib/docker/overlay2  2GiB") || !strings.Contains(out, "3GiB in all") || !strings.Contains(out, "cannot roll back") {
		t.Errorf("cleanup: %d %v\n%s", code, m.made, out)
	}
	if code, _ := moving(m, "cleanup-old-store", "--yes"); code != 0 || len(m.made) != 1 || m.made[0] != "cleanup" {
		t.Errorf("cleanup --yes: %d %v", code, m.made)
	}
}
