// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan_test

// Verifies:
//   - A classic-store host reports its store, compose projects with their scripts, containers not
//     from compose, images in use and unused, and disk; findings come most serious first
//   - Too little room under /var/lib/containerd, swarm and userns-remap block a migration
//   - A containerd-store host says every registry goes through limen, and nothing about a
//     migration it does not need
//   - A project with hooks says so; one with scripts and no hooks suggests them; unreadable hooks warn
//   - A compose that cannot run blocks a migration, unless hooks stop and start every running project
//   - Against a limen: what is not pointed at it yet, or that it cannot be reached

import (
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
)

// containerdRoot is the system containerd's data root, where the new store lives on these hosts.
const containerdRoot = "/var/lib/containerd"

// compose labels a container as part of a compose project in dir.
func compose(project, dir string) map[string]string {
	return map[string]string{
		"com.docker.compose.project":              project,
		"com.docker.compose.project.working_dir":  dir,
		"com.docker.compose.project.config_files": dir + "/docker-compose.yaml",
	}
}

// dnsHost is a host serving DNS from a container: classic store, the resolver on the host's network, resolving
// through itself, with scripts beside its compose file.
func dnsHost() plan.Facts {
	return plan.Facts{
		Engine: host.Info{Version: "29.9.2", Driver: "overlay2", RootDir: "/var/lib/docker"},
		Containers: []host.Container{
			{Name: "resolver", ImageID: "sha256:a", Running: true, HostNetwork: true, Labels: compose("dns", "/srv/dns")},
			{Name: "portainer_agent", ImageID: "sha256:p", Running: true, Ports: []int{9001}, Labels: compose("dns", "/srv/dns")},
			{Name: "oneoff", ImageID: "sha256:o", Labels: map[string]string{}},
		},
		Images: []host.Image{
			{ID: "sha256:a", Size: 80 << 20}, {ID: "sha256:p", Size: 150 << 20}, {ID: "sha256:o", Size: 10 << 20}, {ID: "sha256:old", Size: 70 << 20},
		},
		Nameservers: []string{"127.0.0.1"},
		Free:        map[string]int64{"/var/lib/docker": 13 << 30, containerdRoot: 13 << 30},
		Scripts:     map[string][]string{"dns": {"start.sh", "stop.sh", "update.sh"}},
	}
}

// has reports whether some finding of level holds text.
func has(r hostplan.Report, level, text string) bool {
	return slices.ContainsFunc(r.Findings, func(f hostplan.Finding) bool { return f.Level == level && strings.Contains(f.Text, text) })
}

func TestAClassicHostIsReportedInFull(t *testing.T) {
	r := plan.Decide(dnsHost())
	if r.Store != "classic" || r.Version != "29.9.2" {
		t.Errorf("store %s, version %s", r.Store, r.Version)
	}
	if len(r.Projects) != 1 {
		t.Fatalf("projects %+v", r.Projects)
	}
	p := r.Projects[0]
	if p.Name != "dns" || p.Dir != "/srv/dns" || p.Running != 2 || p.Total != 2 || len(p.Scripts) != 3 || p.Files[0] != "/srv/dns/docker-compose.yaml" {
		t.Errorf("project %+v", p)
	}
	if len(r.Loose) != 1 || r.Loose[0] != "oneoff" {
		t.Errorf("loose %v", r.Loose)
	}
	if r.InUse.Count != 3 || r.InUse.Bytes != 240<<20 || r.Unused.Count != 1 || r.Unused.Bytes != 70<<20 {
		t.Errorf("in use %+v, unused %+v", r.InUse, r.Unused)
	}
	if len(r.Disk) != 2 || r.Disk[1].Path != containerdRoot || r.Disk[1].Free != 13<<30 {
		t.Errorf("disk %+v", r.Disk)
	}
	for _, want := range []struct{ level, text string }{
		{hostplan.Info, "classic image store"},
		{hostplan.Info, "1 images no container uses (up to 70MiB)"},
		{hostplan.Warn, "not from compose (oneoff)"},
		{hostplan.Warn, "perhaps served by a container on the host's network (resolver)"},
		{hostplan.Info, "scripts beside it (start.sh, stop.sh, update.sh)"},
		{hostplan.Info, "no limen given"},
	} {
		if !has(r, want.level, want.text) {
			t.Errorf("no %s finding %q in %+v", want.level, want.text, r.Findings)
		}
	}
	for i := 1; i < len(r.Findings); i++ {
		if r.Findings[i-1].Level == hostplan.Info && r.Findings[i].Level != hostplan.Info {
			t.Errorf("findings out of order: %+v", r.Findings)
		}
	}
}

func TestWhatWouldStopAMigrationBlocksIt(t *testing.T) {
	f := dnsHost()
	f.Free[containerdRoot] = 100 << 20
	f.Engine.Swarm, f.Engine.UsernsRemap = true, true
	r := plan.Decide(f)
	for _, text := range []string{"about 480MiB free on /var/lib/containerd", "swarm mode", "userns-remap"} {
		if !has(r, hostplan.Block, text) {
			t.Errorf("no block %q in %+v", text, r.Findings)
		}
	}
	if r.Findings[0].Level != hostplan.Block {
		t.Errorf("blocks are not first: %+v", r.Findings)
	}
}

func TestAContainerdHostGoesThroughLimenForEveryRegistry(t *testing.T) {
	f := dnsHost()
	f.Engine.Snapshotter = true
	r := plan.Decide(f)
	if r.Store != "containerd" || !has(r, hostplan.Info, "every registry") || has(r, hostplan.Info, "classic image store") {
		t.Errorf("%s %+v", r.Store, r.Findings)
	}
	if has(r, hostplan.Warn, "not from compose") || has(r, hostplan.Info, "scripts beside it") {
		t.Errorf("migration findings on the containerd store: %+v", r.Findings)
	}
}

func TestAHostIsCheckedAgainstALimen(t *testing.T) {
	f := dnsHost()
	f.Limen, f.Registries, f.Configured = "http://10.0.0.5:5000", []string{"docker.io", "ghcr.io"}, []string{"docker.io"}
	r := plan.Decide(f)
	if !has(r, hostplan.Warn, "not pointed at limen yet for ghcr.io") || !has(r, hostplan.Warn, "registry-mirrors does not name limen") {
		t.Errorf("%+v", r.Findings)
	}
	f.Configured, f.Engine.Mirrors = []string{"docker.io", "ghcr.io"}, []string{"http://10.0.0.5:5000/"}
	if r := plan.Decide(f); !has(r, hostplan.Info, "pointed at limen for every registry") {
		t.Errorf("configured: %+v", r.Findings)
	}
	f.LimenErr = "connection refused"
	if r := plan.Decide(f); !has(r, hostplan.Warn, "cannot be reached: connection refused") {
		t.Errorf("unreachable: %+v", r.Findings)
	}
}

func TestHooksAreReported(t *testing.T) {
	f := dnsHost()
	f.Hooks = map[string]hooks.Hooks{"dns": {Stop: "/srv/dns/stop.sh"}}
	r := plan.Decide(f)
	if !has(r, hostplan.Info, "project dns stops through /srv/dns/stop.sh and starts through docker compose up") || has(r, hostplan.Info, "scripts beside it") {
		t.Errorf("hooked: %+v", r.Findings)
	}
	f.Hooks, f.HooksErr = nil, "/etc/limenctl.json: invalid"
	if r := plan.Decide(f); !has(r, hostplan.Warn, "hooks cannot be used: /etc/limenctl.json: invalid") || !has(r, hostplan.Info, "scripts beside it") {
		t.Errorf("unreadable: %+v", r.Findings)
	}
}

func TestAContainerdHostNeedsNoMirrorSetting(t *testing.T) {
	f := dnsHost()
	f.Engine.Snapshotter = true
	f.Limen, f.Registries, f.Configured = "http://192.0.2.10:5000", []string{"docker.io"}, []string{"docker.io"}
	if r := plan.Decide(f); has(r, hostplan.Warn, "registry-mirrors") || !has(r, hostplan.Info, "pointed at limen for every registry") {
		t.Errorf("%+v", r.Findings)
	}
}

func TestAMissingComposeBlocksAMigration(t *testing.T) {
	f := dnsHost()
	f.ComposeErr = "'compose' is not a docker command"
	if r := plan.Decide(f); !has(r, hostplan.Block, "docker compose (v2) cannot run") {
		t.Errorf("%+v", r.Findings)
	}
	f.Hooks = map[string]hooks.Hooks{"dns": {Stop: "/srv/dns/stop.sh", Start: "/srv/dns/start.sh"}}
	if r := plan.Decide(f); has(r, hostplan.Block, "docker compose") {
		t.Errorf("hooked: %+v", r.Findings)
	}
	f.Engine.Snapshotter = true
	f.Hooks = nil
	if r := plan.Decide(f); has(r, hostplan.Block, "docker compose") {
		t.Errorf("containerd store, no migration: %+v", r.Findings)
	}
}
