// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migration_test

// Verifies:
//   - A journal survives being written and read
//   - Rollback puts daemon.json back as the journal recorded it ({} when there was none), with the
//     same projects; nothing recorded, rolled back already, or cleaned up refuses
//   - Cleanup removes the classic store's layers and image metadata and the folders of containers
//     the engine does not know; it refuses on the classic store and without a finished migration
//     unless forced

import (
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/migration"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// journal is a finished migration of one project.
func journal(state string, daemon []byte) *migration.Journal {
	return &migration.Journal{State: state, Started: "2026-10-29T03:00:00Z", Daemon: daemon,
		Projects: []hostplan.MigrateProject{{Name: "dns", Dir: "/srv/dns", Stop: "/srv/dns/stop.sh"}}}
}

func TestAJournalSurvivesWritingAndReading(t *testing.T) {
	j := journal(migration.Switched, []byte(`{"log-driver":"local"}`))
	got, err := migration.Decode(migration.Encode(*j))
	if err != nil || got.State != j.State || string(got.Daemon) != string(j.Daemon) || got.Projects[0].Stop != j.Projects[0].Stop {
		t.Errorf("%+v %v", got, err)
	}
	if _, err := migration.Decode([]byte("{")); errkind.Of(err) != errkind.Invalid {
		t.Errorf("garbled: %v", err)
	}
}

func TestRollbackPutsDaemonJSONBack(t *testing.T) {
	now := []byte(`{"features":{"containerd-snapshotter":true}}`)
	m := migration.Rollback(journal(migration.Migrated, []byte(`{"log-driver":"local"}`)), "/etc/docker/daemon.json", now)
	if len(m.Refusals) != 0 || m.To != hostplan.Classic || len(m.Projects) != 1 {
		t.Fatalf("%+v", m)
	}
	if w := m.Writes[0]; w.Path != "/etc/docker/daemon.json" || string(w.Before) != string(now) || string(w.After) != `{"log-driver":"local"}` {
		t.Errorf("%+v", w)
	}
	if m := migration.Rollback(journal(migration.Switched, nil), "/etc/docker/daemon.json", now); string(m.Writes[0].After) != "{}\n" {
		t.Errorf("no daemon.json before: %q", m.Writes[0].After)
	}
	for _, j := range []*migration.Journal{nil, journal(migration.RolledBack, nil), journal(migration.Cleaned, nil)} {
		if m := migration.Rollback(j, "/etc/docker/daemon.json", now); len(m.Refusals) == 0 {
			t.Errorf("%+v: not refused", j)
		}
	}
}

func TestCleanupTakesOnlyTheClassicStore(t *testing.T) {
	e := host.Info{RootDir: "/var/lib/docker", Snapshotter: true}
	c := migration.Cleanup(e, journal(migration.Migrated, nil), []string{"new1"}, []string{"new1", "old1", "old2"}, false)
	var paths []string
	for _, p := range c.Paths {
		paths = append(paths, p.Path)
	}
	want := []string{"/var/lib/docker/overlay2", "/var/lib/docker/image/overlay2", "/var/lib/docker/containers/old1", "/var/lib/docker/containers/old2"}
	if len(c.Refusals) != 0 || !slices.Equal(paths, want) {
		t.Errorf("%v %v", c.Refusals, paths)
	}
}

func TestCleanupRefusesWhereItWouldHarm(t *testing.T) {
	if c := migration.Cleanup(host.Info{RootDir: "/var/lib/docker"}, journal(migration.Migrated, nil), nil, nil, true); len(c.Refusals) == 0 || len(c.Paths) != 0 {
		t.Errorf("classic store: %+v", c)
	}
	if c := migration.Cleanup(host.Info{RootDir: "relative", Snapshotter: true}, nil, nil, nil, true); len(c.Refusals) == 0 || len(c.Paths) != 0 {
		t.Errorf("relative root: %+v", c)
	}
	e := host.Info{RootDir: "/var/lib/docker", Snapshotter: true}
	if c := migration.Cleanup(e, journal(migration.Switched, nil), nil, nil, false); len(c.Refusals) == 0 {
		t.Errorf("unfinished: %+v", c)
	}
	if c := migration.Cleanup(e, nil, nil, nil, true); len(c.Refusals) != 0 || len(c.Paths) != 2 {
		t.Errorf("forced by hand: %+v", c)
	}
}
