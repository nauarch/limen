// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migrate_test

// Verifies:
//   - A migration saves the images built on the host first and loads them back after the switch;
//   - A migration fills limen first, stops the projects (DNS last), writes daemon.json with the
//     store on and limen as mirror, restarts Docker, starts the projects (DNS first), and records
//     each stage in the journal
//   - limen failing to fetch stops it before anything stops
//   - Docker coming back on the classic store, or a project failing to start, leaves the journal
//     saying how far it got; rollback then stops, puts daemon.json back, restarts and starts
//   - A migration stopped half way refuses a new one; a refused plan is never made
//   - Docker Desktop is refused every move between stores, which it makes in its own settings

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/features/hosts/app/configure"
	"github.com/nauarch/limen/internal/features/hosts/app/migrate"
	"github.com/nauarch/limen/internal/features/hosts/app/report"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	"github.com/nauarch/limen/internal/features/hosts/domain/migration"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	fake "github.com/nauarch/limen/tests/kit/host"
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

// classic is the engine before the move.
var classic = host.Info{Version: "29.9.2", RootDir: "/var/lib/docker", OS: "linux", Arch: "aarch64", Containerd: "/run/containerd/containerd.sock"}

// rig is a classic arm64 host with a dns project (hooked, on the host's network) and a web project.
type rig struct {
	m      *migrate.Migrator
	engine *fake.Engine
	files  *fake.Files
	limen  *fake.Limen
	log    *fake.Log
}

func newRig() *rig {
	containerd := classic
	containerd.Snapshotter = true
	r := &rig{
		engine: &fake.Engine{
			Current: classic, AfterRestart: &containerd,
			ContainerList: []host.Container{
				{Name: "resolver", Image: "coredns/coredns:latest", ImageID: "sha256:ag", Running: true, HostNetwork: true, Labels: in("dns")},
				{Name: "web-1", Image: "nginx:alpine", ImageID: "sha256:ng", Running: true, Labels: in("web")},
			},
			ImageList: []host.Image{
				{ID: "sha256:ag", Digests: []string{"coredns/coredns@sha256:1"}, Size: 1},
				{ID: "sha256:ng", Digests: []string{"nginx@sha256:2"}, Size: 1},
				{ID: "sha256:old", Tags: []string{"web:previous"}, Size: 1},
			},
		},
		files: fake.NewFiles(map[string]string{
			layout.Linux().DaemonJSON: `{"log-driver":"local"}`,
			hooks.Path:                `{"projects":{"dns":{"stop":"/srv/dns/stop.sh","start":"/srv/dns/start.sh"}}}`,
			"/etc/resolv.conf":        "nameserver 192.0.2.54\n",
		}),
		limen: &fake.Limen{Served: []string{"docker.io"}},
		log:   &fake.Log{},
	}
	services := &fake.Services{Engine: r.engine, Log: r.log}
	clock := kitclock.At(time.Date(2026, 10, 29, 3, 0, 0, 0, time.UTC))
	compose := &fake.Compose{Log: r.log}
	r.m = &migrate.Migrator{
		Gatherer:   &report.Gatherer{Layout: layout.Linux(), Engine: r.engine, Files: r.files, Network: fake.Network{}, Compose: compose, Limen: r.limen.Client},
		Configurer: &configure.Configurer{Layout: layout.Linux(), Engine: r.engine, Files: r.files, Services: services, Limen: r.limen.Client, Clock: clock},
		Engine:     r.engine, Files: r.files, Services: services,
		Compose: compose, Scripts: &fake.Scripts{Log: r.log},
		Limen: r.limen.Client, Clock: clock, PrefetchWait: 10 * time.Second, PrefetchPoll: time.Millisecond,
	}
	return r
}

// state is the journal's state, "" when there is none.
func (r *rig) state(t *testing.T) string {
	data, ok := r.files.Data[migration.JournalPath]
	if !ok {
		return ""
	}
	j, err := migration.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return j.State
}

// run plans and makes a migration, giving the steps it reported.
func (r *rig) run(t *testing.T, req hostplan.MigrateRequest) ([]string, error) {
	t.Helper()
	plan, err := r.m.PlanMigration(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Refusals) != 0 {
		t.Fatalf("refused: %v", plan.Refusals)
	}
	var steps []string
	return steps, r.m.Migrate(context.Background(), plan, func(s string) { steps = append(steps, s) })
}

func TestAMigrationMovesTheHost(t *testing.T) {
	r := newRig()
	steps, err := r.run(t, hostplan.MigrateRequest{Limen: limen})
	if err != nil {
		t.Fatalf("%v (steps %v)", err, steps)
	}
	want := []string{"stop web", "run /srv/dns/stop.sh", "restart docker", "run /srv/dns/start.sh", "up web"}
	if !slices.Equal(r.log.Events, want) {
		t.Errorf("events %v", r.log.Events)
	}
	if len(r.limen.Asked) != 1 || !slices.Equal(r.limen.Asked[0].Platforms, []string{"linux/arm64"}) || len(r.limen.Asked[0].Images) != 2 {
		t.Errorf("limen asked %+v", r.limen.Asked)
	}
	daemon := string(r.files.Data[layout.Linux().DaemonJSON])
	for _, want := range []string{`"containerd-snapshotter": true`, limen, `"log-driver": "local"`} {
		if !strings.Contains(daemon, want) {
			t.Errorf("daemon.json lacks %s:\n%s", want, daemon)
		}
	}
	if _, ok := r.files.Data["/etc/docker/certs.d/docker.io/hosts.toml"]; !ok {
		t.Error("hosts.toml not written")
	}
	if !slices.Contains(r.files.Made, "/var/lib/limenctl") {
		t.Errorf("the folder for the saved images was not made: %v", r.files.Made)
	}
	if !slices.Equal(r.engine.Saved, []string{"web:previous"}) || r.engine.Loaded != migration.KeepPath || !slices.Contains(r.files.Removed, migration.KeepPath) {
		t.Errorf("saved %v, loaded %q, removed %v", r.engine.Saved, r.engine.Loaded, r.files.Removed)
	}
	if got := r.state(t); got != migration.Migrated {
		t.Errorf("journal %s", got)
	}
}

func TestLimenFailingToFetchStopsNothing(t *testing.T) {
	r := newRig()
	r.limen.Fetched = &prefetch.Job{ID: "j1", State: prefetch.Failed, Failures: []string{"nginx:alpine: not found"}}
	_, err := r.run(t, hostplan.MigrateRequest{Limen: limen})
	if errkind.Of(err) != errkind.Upstream || !strings.Contains(err.Error(), "nothing was stopped") {
		t.Errorf("%v", err)
	}
	if len(r.log.Events) != 0 || r.state(t) != "" {
		t.Errorf("events %v, journal %q", r.log.Events, r.state(t))
	}
}

func TestDockerStayingClassicIsCaught(t *testing.T) {
	r := newRig()
	r.engine.AfterRestart = &classic
	_, err := r.run(t, hostplan.MigrateRequest{Limen: limen})
	if err == nil || !strings.Contains(err.Error(), "wrong image store") || r.state(t) != migration.Stopping {
		t.Errorf("%v, journal %s", err, r.state(t))
	}
	if r.log.Events[len(r.log.Events)-1] != "restart docker" {
		t.Errorf("went on: %v", r.log.Events)
	}
}

func TestAFailedStartRollsBack(t *testing.T) {
	r := newRig()
	r.m.Compose = &fake.Compose{Log: r.log, Fail: "up web"}
	if _, err := r.run(t, hostplan.MigrateRequest{Limen: limen}); err == nil || !strings.Contains(err.Error(), "limenctl rollback") {
		t.Fatalf("%v", err)
	}
	if got := r.state(t); got != migration.Switched {
		t.Fatalf("journal %s", got)
	}
	if plan, _ := r.m.PlanMigration(context.Background(), hostplan.MigrateRequest{Limen: limen}); len(plan.Refusals) == 0 {
		t.Error("a new migration was not refused half way")
	}

	r.m.Compose = &fake.Compose{Log: r.log}
	r.engine.AfterRestart = &classic
	r.log.Events = nil
	back, err := r.m.PlanRollback(context.Background())
	if err != nil || len(back.Refusals) != 0 {
		t.Fatalf("%+v %v", back, err)
	}
	if err := r.m.Rollback(context.Background(), back, func(string) {}); err != nil {
		t.Fatal(err)
	}
	want := []string{"stop web", "run /srv/dns/stop.sh", "restart docker", "run /srv/dns/start.sh", "up web"}
	if !slices.Equal(r.log.Events, want) {
		t.Errorf("events %v", r.log.Events)
	}
	if got := string(r.files.Data[layout.Linux().DaemonJSON]); got != `{"log-driver":"local"}` || r.state(t) != migration.RolledBack {
		t.Errorf("daemon.json %s, journal %s", got, r.state(t))
	}
}

func TestARefusedPlanIsNeverMade(t *testing.T) {
	r := newRig()
	err := r.m.Migrate(context.Background(), hostplan.Migration{Refusals: []string{"no"}}, func(string) {})
	if errkind.Of(err) != errkind.Conflict || len(r.log.Events) != 0 {
		t.Errorf("%v %v", err, r.log.Events)
	}
	if err := r.m.Rollback(context.Background(), hostplan.Migration{}, func(string) {}); errkind.Of(err) != errkind.NotFound {
		t.Errorf("rollback without a journal: %v", err)
	}
}

func TestRollbackPassesProjectsItCannotStop(t *testing.T) {
	r := newRig()
	r.engine.AfterRestart = &classic
	if _, err := r.run(t, hostplan.MigrateRequest{Limen: limen}); err == nil {
		t.Fatal("a migration with Docker staying classic passed")
	}
	r.m.Compose = &fake.Compose{Log: r.log, Fail: "stop web"}
	back, err := r.m.PlanRollback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	if err := r.m.Rollback(context.Background(), back, func(s string) { steps = append(steps, s) }); err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(steps, func(s string) bool { return strings.HasPrefix(s, "could not stop web") }) || r.state(t) != migration.RolledBack {
		t.Errorf("steps %v, journal %s", steps, r.state(t))
	}
}

func TestDockerDesktopIsRefusedEveryMove(t *testing.T) {
	r := newRig()
	r.m.Configurer.Layout = layout.Desktop("C:/Users/me")
	ctx := context.Background()
	m, err := r.m.PlanMigration(ctx, hostplan.MigrateRequest{Limen: limen})
	if err != nil || len(m.Refusals) != 1 || !strings.Contains(m.Refusals[0], "Docker Desktop") {
		t.Errorf("migrate: %+v %v", m.Refusals, err)
	}
	if back, err := r.m.PlanRollback(ctx); err != nil || len(back.Refusals) != 1 {
		t.Errorf("rollback: %+v %v", back.Refusals, err)
	}
	if c, err := r.m.PlanCleanup(ctx, true); err != nil || len(c.Refusals) != 1 {
		t.Errorf("cleanup: %+v %v", c.Refusals, err)
	}
	if len(r.log.Events) != 0 || len(r.files.Written) != 0 {
		t.Errorf("touched the host: %v %v", r.log.Events, r.files.Written)
	}
}
