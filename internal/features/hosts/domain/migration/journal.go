// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// JournalPath is where a migration records how far it got, for rollback and cleanup to read.
const JournalPath = "/var/lib/limenctl/migration.json"

// How far a migration got.
const (
	// Stopping: the projects are being stopped; Docker has not switched yet.
	Stopping = "stopping"
	// Switched: Docker runs on the containerd store; the projects are being started.
	Switched = "switched"
	// Migrated: every project runs on the containerd store.
	Migrated = "migrated"
	// RolledBack: the host is back on the classic store.
	RolledBack = "rolled-back"
	// Cleaned: the classic store's data is gone; there is no way back.
	Cleaned = "cleaned"
)

// Journal is a migration's record: how far it got, when it started, the projects it stopped, and
// daemon.json as it was before (Daemon nil when there was none).
type Journal struct {
	State    string                    `json:"state"`
	Started  string                    `json:"started"`
	Projects []hostplan.MigrateProject `json:"projects"`
	Daemon   []byte                    `json:"daemon"`
}

// Encode writes a journal.
func Encode(j Journal) []byte {
	out, _ := json.MarshalIndent(j, "", "  ")
	return append(out, '\n')
}

// Decode reads a journal.
func Decode(data []byte) (Journal, error) {
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return Journal{}, fmt.Errorf("%s: %w: %w", JournalPath, errkind.ErrInvalid, err)
	}
	return j, nil
}

// Rollback works out the way back to the classic store from a journal (nil when none was found):
// stop the projects, put daemon.json back as it was, start them again on the classic store,
// where their old containers wait. daemon is daemon.json, at daemonPath, as it is now.
func Rollback(j *Journal, daemonPath string, daemon []byte) hostplan.Migration {
	m := hostplan.Migration{To: hostplan.Classic}
	switch {
	case j == nil:
		m.Refusals = append(m.Refusals, "no migration is recorded in "+JournalPath)
		return m
	case j.State == Cleaned:
		m.Refusals = append(m.Refusals, "the classic store was cleaned up: there is nothing to go back to")
	case j.State == RolledBack:
		m.Refusals = append(m.Refusals, "the migration was rolled back already")
	}
	m.Projects = j.Projects
	before := j.Daemon
	if before == nil {
		before = []byte("{}\n")
	}
	m.Writes = []hostplan.Write{{Path: daemonPath, Before: daemon, After: before}}
	m.Notes = append(m.Notes, "containers started on the containerd store stay there, stopped; their images too, until a migration or cleanup")
	return m
}

// Cleanup works out the classic store's data to remove: its layers, its image metadata, and the
// folders of containers the running (containerd-store) engine does not know. known are the IDs
// of the containers the engine lists, containerDirs the entries of its containers folder; force
// goes ahead without a finished migration on record (for hosts moved by hand).
func Cleanup(e host.Info, j *Journal, known, containerDirs []string, force bool) hostplan.Cleanup {
	var c hostplan.Cleanup
	if !e.Snapshotter {
		c.Refusals = append(c.Refusals, "the host is on the classic store: cleaning it would remove the images it runs")
		return c
	}
	if e.RootDir == "" || !path.IsAbs(e.RootDir) {
		c.Refusals = append(c.Refusals, fmt.Sprintf("Docker's root %q is not an absolute path", e.RootDir))
		return c
	}
	if (j == nil || j.State != Migrated) && !force {
		state := "none"
		if j != nil {
			state = j.State
		}
		c.Refusals = append(c.Refusals, "no finished migration is recorded (state "+state+"): --force cleans up a host moved by hand")
	}
	for _, p := range []string{"overlay2", "image/overlay2"} {
		c.Paths = append(c.Paths, hostplan.Removal{Path: path.Join(e.RootDir, p)})
	}
	for _, id := range containerDirs {
		if !slices.Contains(known, id) {
			c.Paths = append(c.Paths, hostplan.Removal{Path: path.Join(e.RootDir, "containers", id)})
		}
	}
	return c
}
