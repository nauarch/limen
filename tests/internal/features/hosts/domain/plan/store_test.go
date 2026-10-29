// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan_test

// Verifies:
//   - The new store lives in the system containerd's root, /var/lib/containerd unless its
//     config.toml moves it, or under Docker's root for a containerd dockerd runs itself
//   - The free space checked before a migration is that root's
//   - Containers holding a megabyte or more outside their volumes are named; less is not

import (
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
)

func TestTheStoreRootFollowsTheContainerdInUse(t *testing.T) {
	system := host.Info{RootDir: "/var/lib/docker", Containerd: "/run/containerd/containerd.sock"}
	managed := host.Info{RootDir: "/srv/docker/", Containerd: "/var/run/docker/containerd/containerd.sock"}
	moved := []byte("version = 2\nroot = \"/data/containerd\"\n\n[plugins]\n  root = \"/elsewhere\"\n")
	for _, c := range []struct {
		engine host.Info
		config []byte
		want   string
	}{
		{system, nil, "/var/lib/containerd"},
		{host.Info{RootDir: "/var/lib/docker"}, nil, "/var/lib/containerd"},
		{system, moved, "/data/containerd"},
		{system, []byte("[plugins]\nroot = \"/elsewhere\"\n"), "/var/lib/containerd"},
		{managed, moved, "/srv/docker/containerd"},
	} {
		if got := plan.StoreRoot(c.engine, c.config); got != c.want {
			t.Errorf("%+v %q: %s, want %s", c.engine, c.config, got, c.want)
		}
	}
}

func TestRoomIsCheckedWhereTheStoreWillLive(t *testing.T) {
	f := dnsHost()
	f.Engine.Containerd = "/var/run/docker/containerd/containerd.sock"
	f.Free["/var/lib/docker/containerd"] = 100 << 20
	r := plan.Decide(f)
	if !has(r, hostplan.Block, "free on /var/lib/docker/containerd") || r.Disk[1].Path != "/var/lib/docker/containerd" {
		t.Errorf("%+v %+v", r.Disk, r.Findings)
	}
}

func TestDataOutsideVolumesIsNamed(t *testing.T) {
	f := dnsHost()
	f.Containers[0].Written = 3 << 20
	f.Containers[1].Written = 4096
	r := plan.Decide(f)
	if !has(r, hostplan.Warn, "outside their volumes (resolver 3MiB)") || has(r, hostplan.Warn, "portainer_agent 4") {
		t.Errorf("%+v", r.Findings)
	}
}

func TestDataOutsideVolumesMattersOnlyBeforeAMigration(t *testing.T) {
	f := dnsHost()
	f.Engine.Snapshotter = true
	f.Containers[0].Written = 3 << 20
	if r := plan.Decide(f); has(r, hostplan.Warn, "outside their volumes") {
		t.Errorf("%+v", r.Findings)
	}
}
