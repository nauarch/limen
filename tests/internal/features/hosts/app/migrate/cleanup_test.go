// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migrate_test

// Verifies:
//   - Cleanup lists the classic store's data with its size, leaving out what is gone, and keeps
//     the containers the engine knows
//   - Removing it records that there is no way back, so rollback refuses
//   - A refused cleanup removes nothing

import (
	"context"
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/migration"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// migrated is the rig after a migration, with the classic store's data still on disk.
func migrated(t *testing.T) *rig {
	r := newRig()
	if _, err := r.run(t, hostplan.MigrateRequest{Limen: limen}); err != nil {
		t.Fatal(err)
	}
	r.files.Removed = nil
	r.engine.ContainerList = []host.Container{{ID: "new1", Name: "web-1"}}
	for p, d := range map[string]string{
		"/var/lib/docker/overlay2/l1/diff/bin/sh":        "12345",
		"/var/lib/docker/containers/old1/config.v2.json": "123",
		"/var/lib/docker/containers/new1/config.v2.json": "1",
		"/var/lib/docker/volumes/data/_data/kept.txt":    "kept",
	} {
		r.files.Data[p] = []byte(d)
	}
	return r
}

func TestCleanupRemovesOnlyTheClassicStore(t *testing.T) {
	r := migrated(t)
	c, err := r.m.PlanCleanup(context.Background(), false)
	if err != nil || len(c.Refusals) != 0 {
		t.Fatalf("%+v %v", c, err)
	}
	want := []hostplan.Removal{{Path: "/var/lib/docker/overlay2", Bytes: 5}, {Path: "/var/lib/docker/containers/old1", Bytes: 3}}
	if !slices.Equal(c.Paths, want) {
		t.Fatalf("%+v", c.Paths)
	}
	if err := r.m.Cleanup(context.Background(), c, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.files.Removed, []string{"/var/lib/docker/overlay2", "/var/lib/docker/containers/old1"}) {
		t.Errorf("removed %v", r.files.Removed)
	}
	if _, ok := r.files.Data["/var/lib/docker/volumes/data/_data/kept.txt"]; !ok || r.state(t) != migration.Cleaned {
		t.Errorf("volume kept %v, journal %s", ok, r.state(t))
	}
	if back, _ := r.m.PlanRollback(context.Background()); len(back.Refusals) == 0 {
		t.Error("rollback after cleanup was not refused")
	}
}

func TestARefusedCleanupRemovesNothing(t *testing.T) {
	r := newRig()
	c, err := r.m.PlanCleanup(context.Background(), true)
	if err != nil || len(c.Refusals) == 0 {
		t.Fatalf("classic host: %+v %v", c, err)
	}
	if err := r.m.Cleanup(context.Background(), c, func(string) {}); errkind.Of(err) != errkind.Conflict || len(r.files.Removed) != 0 {
		t.Errorf("%v %v", err, r.files.Removed)
	}
}
