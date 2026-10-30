// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package report_test

// Verifies:
//   - A host's engine, resolv.conf, own addresses, free space and the scripts beside each compose
//     project are read
//   - Given a limen, the registries it serves and which hosts.toml files point at it are read
//   - A limen that does not answer is noted, not an error
//   - An engine that does not answer is an error
//   - A compose plugin that cannot run is noted with why
//   - Hooks naming a script that is missing or not executable are noted with why, and none kept

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts/app/report"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/features/hosts/domain/hoststoml"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	fake "github.com/nauarch/limen/tests/kit/host"
)

const limen = "http://10.0.0.5:5000"

// containerdRoot is the system containerd's data root, where the new store lives on this host.
const containerdRoot = "/var/lib/containerd"

// gatherer reads a host with one compose project in /srv/dns.
func gatherer(l *fake.Limen) (*report.Gatherer, *fake.Engine, *fake.Files) {
	e := &fake.Engine{
		Current: host.Info{RootDir: "/var/lib/docker"},
		ContainerList: []host.Container{{Name: "resolver", Labels: map[string]string{
			"com.docker.compose.project": "dns", "com.docker.compose.project.working_dir": "/srv/dns",
		}}},
	}
	f := fake.NewFiles(map[string]string{
		"/etc/resolv.conf":                    "nameserver 127.0.0.1\n",
		"/srv/dns/docker-compose.yaml":        "services: {}\n",
		"/srv/dns/stop.sh":                    "",
		"/srv/dns/start.sh":                   "",
		"/srv/dns/conf/Corefile":              "",
		layout.Linux().HostsToml("docker.io"): string(hoststoml.Render("docker.io", limen)),
		layout.Linux().HostsToml("ghcr.io"):   "server = \"https://ghcr.io\"\n",
	})
	f.FreeAt["/var/lib/docker"], f.FreeAt[containerdRoot] = 10<<30, 20<<30
	return &report.Gatherer{Layout: layout.Linux(), Engine: e, Files: f, Network: fake.Network{Own: []string{"127.0.0.1", "192.0.2.53"}}, Compose: &fake.Compose{}, Limen: l.Client}, e, f
}

func TestAHostIsRead(t *testing.T) {
	g, _, _ := gatherer(&fake.Limen{})
	f, err := g.Gather(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.Addresses, []string{"127.0.0.1", "192.0.2.53"}) {
		t.Errorf("addresses %v", f.Addresses)
	}
	if !slices.Equal(f.Nameservers, []string{"127.0.0.1"}) || f.Free["/var/lib/docker"] != 10<<30 || f.Free[containerdRoot] != 20<<30 {
		t.Errorf("nameservers %v, free %v", f.Nameservers, f.Free)
	}
	if !slices.Equal(f.Scripts["dns"], []string{"start.sh", "stop.sh"}) {
		t.Errorf("scripts %v", f.Scripts)
	}
	if f.Limen != "" || f.Registries != nil {
		t.Errorf("checked against a limen: %+v", f)
	}
}

func TestAHostIsCheckedAgainstALimen(t *testing.T) {
	g, _, _ := gatherer(&fake.Limen{Served: []string{"docker.io", "ghcr.io", "quay.io"}})
	f, err := g.Gather(context.Background(), limen)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Registries) != 3 || !slices.Equal(f.Configured, []string{"docker.io"}) || f.LimenErr != "" {
		t.Errorf("registries %v, configured %v, err %q", f.Registries, f.Configured, f.LimenErr)
	}
}

func TestALimenNotAnsweringIsNoted(t *testing.T) {
	g, _, _ := gatherer(&fake.Limen{Health: errors.New("connection refused")})
	f, err := g.Gather(context.Background(), limen)
	if err != nil || f.LimenErr != "connection refused" {
		t.Errorf("%q %v", f.LimenErr, err)
	}
}

func TestAnEngineNotAnsweringIsAnError(t *testing.T) {
	g, e, _ := gatherer(&fake.Limen{})
	e.Err = errors.New("cannot connect to the Docker daemon")
	if _, err := g.Gather(context.Background(), ""); err == nil {
		t.Error("no error")
	}
}

func TestHooksThatCannotRunAreNoted(t *testing.T) {
	g, _, files := gatherer(&fake.Limen{})
	files.Data[hooks.Path] = []byte(`{"projects":{"dns":{"stop":"/srv/dns/stop.sh","start":"/srv/dns/start.sh"}}}`)
	if f, err := g.Gather(context.Background(), ""); err != nil || f.HooksErr != "" || f.Hooks["dns"].Start != "/srv/dns/start.sh" {
		t.Fatalf("runnable hooks: %+v %q %v", f.Hooks, f.HooksErr, err)
	}
	files.Plain = []string{"/srv/dns/start.sh"}
	if f, _ := g.Gather(context.Background(), ""); !strings.Contains(f.HooksErr, "/srv/dns/start.sh is not executable (chmod +x /srv/dns/start.sh)") || f.Hooks != nil {
		t.Errorf("not executable: %+v %q", f.Hooks, f.HooksErr)
	}
	delete(files.Data, "/srv/dns/stop.sh")
	if f, _ := g.Gather(context.Background(), ""); !strings.Contains(f.HooksErr, "/srv/dns/stop.sh does not exist") {
		t.Errorf("missing: %q", f.HooksErr)
	}
}

func TestAMissingComposeIsNoted(t *testing.T) {
	g, _, _ := gatherer(&fake.Limen{})
	if f, err := g.Gather(context.Background(), ""); err != nil || f.ComposeErr != "" {
		t.Fatalf("compose present: %q %v", f.ComposeErr, err)
	}
	g.Compose = &fake.Compose{Missing: true}
	if f, err := g.Gather(context.Background(), ""); err != nil || !strings.Contains(f.ComposeErr, "not a docker command") {
		t.Errorf("compose missing: %q %v", f.ComposeErr, err)
	}
}
