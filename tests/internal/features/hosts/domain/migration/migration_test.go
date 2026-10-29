// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migration_test

// Verifies:
//   - A classic host with its DNS in a project moves with that project started first and stopped
//     last, its owner's hooks used, and limen asked for the pulled images of the host's platform
//   - Images built on the host are kept (saved and loaded back), the unused too, rather than
//     fetched; untagged ones are noted, and a host without room for them refuses; projects not
//     running are left alone
//   - A containerd host, a block, an unreachable limen refuse; containers not from compose, and a
//     host resolving only through itself without a limen, refuse unless forced
//   - Pruning counts the unused images; platforms are named as registries name them

import (
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/features/hosts/domain/migration"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
)

const limen = "http://10.0.0.5:5000"

// in labels a container as part of a compose project in /srv/<project>.
func in(project string) map[string]string {
	return map[string]string{
		"com.docker.compose.project":              project,
		"com.docker.compose.project.working_dir":  "/srv/" + project,
		"com.docker.compose.project.config_files": "/srv/" + project + "/compose.yaml",
	}
}

// host is a classic amd64 host resolving through its own DNS resolver: web (one image built here, one
// pulled) listed before dns, and a stopped project old.
func facts() plan.Facts {
	return plan.Facts{
		Engine: host.Info{Version: "29.9.2", RootDir: "/var/lib/docker", OS: "linux", Arch: "x86_64"},
		Containers: []host.Container{
			{Name: "web-app-1", Image: "web-app", ImageID: "sha256:built", Running: true, Labels: in("web")},
			{Name: "web-db-1", Image: "postgres:17", ImageID: "sha256:pg", Running: true, Labels: in("web")},
			{Name: "resolver", Image: "coredns/coredns:latest", ImageID: "sha256:ag", Running: true, HostNetwork: true, Labels: in("dns")},
			{Name: "old-1", Image: "redis:7", ImageID: "sha256:redis", Labels: in("old")},
		},
		Images: []host.Image{
			{ID: "sha256:built", Tags: []string{"web-app:latest"}, Size: 10},
			{ID: "sha256:previous", Tags: []string{"web-app:previous"}, Size: 10},
			{ID: "sha256:pg", Digests: []string{"postgres@sha256:1"}, Size: 20},
			{ID: "sha256:ag", Digests: []string{"coredns/coredns@sha256:2"}, Size: 30},
			{ID: "sha256:redis", Digests: []string{"redis@sha256:3"}, Size: 40},
			{ID: "sha256:unused", Digests: []string{"alpine@sha256:4"}, Size: 50},
		},
		Nameservers: []string{"127.0.0.1"},
		Free:        map[string]int64{"/var/lib/containerd": 100 << 30},
		Limen:       limen,
	}
}

// decide works the migration out as limenctl does.
func decide(f plan.Facts, owned map[string]hooks.Hooks, r hostplan.MigrateRequest) hostplan.Migration {
	f.Hooks = owned
	return migration.Decide(f, plan.Decide(f), r)
}

// noted reports whether some note holds text.
func noted(m hostplan.Migration, text string) bool {
	return slices.ContainsFunc(m.Notes, func(n string) bool { return strings.Contains(n, text) })
}

// refusedFor reports whether some refusal holds text.
func refusedFor(m hostplan.Migration, text string) bool {
	return slices.ContainsFunc(m.Refusals, func(r string) bool { return strings.Contains(r, text) })
}

func TestAHostMovesWithItsDNSFirst(t *testing.T) {
	owned := map[string]hooks.Hooks{"dns": {Stop: "/srv/dns/stop.sh", Start: "/srv/dns/start.sh"}}
	m := decide(facts(), owned, hostplan.MigrateRequest{Limen: limen})
	if len(m.Refusals) != 0 {
		t.Fatalf("refused: %v", m.Refusals)
	}
	if m.To != hostplan.Containerd || m.Platform != "linux/amd64" || m.Limen != limen {
		t.Errorf("%+v", m)
	}
	if len(m.Projects) != 2 || m.Projects[0].Name != "dns" || m.Projects[1].Name != "web" {
		t.Fatalf("projects %+v", m.Projects)
	}
	if dns := m.Projects[0]; dns.Stop != "/srv/dns/stop.sh" || dns.Start != "/srv/dns/start.sh" || dns.Dir != "/srv/dns" || dns.Files[0] != "/srv/dns/compose.yaml" {
		t.Errorf("dns %+v", dns)
	}
	if !slices.Equal(m.Prefetch, []string{"coredns/coredns:latest", "postgres:17"}) {
		t.Errorf("prefetch %v", m.Prefetch)
	}
	if !slices.Equal(m.Keep, []string{"web-app:latest", "web-app:previous"}) || m.KeepPath != migration.KeepPath || m.KeepBytes != 20 {
		t.Errorf("keep %v at %s, %d bytes", m.Keep, m.KeepPath, m.KeepBytes)
	}
	if !noted(m, "project old is not running") || m.Prune {
		t.Errorf("notes %v, prune %v", m.Notes, m.Prune)
	}
}

func TestWhatRefusesAMove(t *testing.T) {
	f := facts()
	f.Engine.Snapshotter = true
	if m := decide(f, nil, hostplan.MigrateRequest{Limen: limen}); !refusedFor(m, "containerd image store already") {
		t.Errorf("containerd: %v", m.Refusals)
	}
	f = facts()
	f.Engine.Swarm = true
	if m := decide(f, nil, hostplan.MigrateRequest{Limen: limen, Force: true}); !refusedFor(m, "swarm mode") {
		t.Errorf("swarm, even forced: %v", m.Refusals)
	}
	f = facts()
	f.LimenErr = "connection refused"
	if m := decide(f, nil, hostplan.MigrateRequest{Limen: limen}); !refusedFor(m, "cannot be reached: connection refused") {
		t.Errorf("limen down: %v", m.Refusals)
	}
}

func TestWarningsRefuseUnlessForced(t *testing.T) {
	f := facts()
	f.Containers = append(f.Containers, host.Container{Name: "stray", ImageID: "sha256:pg", Running: true, Labels: map[string]string{}})
	f.Limen = ""
	m := decide(f, nil, hostplan.MigrateRequest{})
	if !refusedFor(m, "not from compose (stray)") || !refusedFor(m, "resolves names only through itself") || !refusedFor(m, "--force") {
		t.Errorf("unforced: %v", m.Refusals)
	}
	m = decide(f, nil, hostplan.MigrateRequest{Force: true})
	if len(m.Refusals) != 0 || !noted(m, "forced past: containers not from compose") || !noted(m, "no limen given") {
		t.Errorf("forced: %v %v", m.Refusals, m.Notes)
	}
}

func TestScriptsWithoutHooksAndPartialProjectsAreNoted(t *testing.T) {
	f := facts()
	f.Scripts = map[string][]string{"dns": {"start.sh"}}
	f.Containers = append(f.Containers, host.Container{Name: "web-cache-1", Image: "redis:7", ImageID: "sha256:redis", Labels: in("web")})
	m := decide(f, nil, hostplan.MigrateRequest{Limen: limen})
	if !noted(m, "project dns has scripts beside it but no hooks") || !noted(m, "project web runs 2 of 3 containers") {
		t.Errorf("%v", m.Notes)
	}
}

func TestPruningCountsTheUnusedImages(t *testing.T) {
	m := decide(facts(), nil, hostplan.MigrateRequest{Limen: limen, Prune: true})
	if !m.Prune || m.PruneBytes != 60 {
		t.Errorf("%v %d", m.Prune, m.PruneBytes)
	}
}

func TestPlatformsAreNamedAsRegistriesNameThem(t *testing.T) {
	for arch, want := range map[string]string{"x86_64": "linux/amd64", "aarch64": "linux/arm64", "armv7l": "linux/arm/v7", "riscv64": "linux/riscv64"} {
		if got := migration.Platform(host.Info{OS: "linux", Arch: arch}); got != want {
			t.Errorf("%s: %s", arch, got)
		}
	}
}

func TestBuiltImagesNeedRoomAndATag(t *testing.T) {
	f := facts()
	f.Images[0].Tags = []string{"<none>:<none>"}
	f.Free["/var/lib/docker"] = 5
	m := decide(f, nil, hostplan.MigrateRequest{Limen: limen})
	if !noted(m, "an untagged image built on this host (sha256:built) cannot be kept") || !slices.Equal(m.Keep, []string{"web-app:previous"}) {
		t.Errorf("untagged: %v %v", m.Keep, m.Notes)
	}
	if !refusedFor(m, "needs about 10B free for /var/lib/limenctl/images.tar") {
		t.Errorf("no room: %v", m.Refusals)
	}
}
