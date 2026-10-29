// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package configure_test

// Verifies:
//   - Planning writes a hosts.toml per registry limen serves and puts limen in daemon.json,
//     which needs a restart
//   - A host already pointed at limen needs nothing
//   - On the containerd image store daemon.json is left alone, so Docker does not restart
//   - A hosts.toml limenctl did not write is skipped unless forced
//   - A limen that is not an http(s) URL, or does not answer, is refused
//   - Applying backs up what it replaces, writes, restarts Docker and checks it took the mirror
//   - Docker coming back without the mirror is an error; without a restart Docker is left alone
//   - On Docker Desktop every change restarts it, as it reads hosts.toml only when it starts, and
//     no mirror is looked for when daemon.json was not written

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/app/configure"
	"github.com/nauarch/limen/internal/features/hosts/domain/hoststoml"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	fake "github.com/nauarch/limen/tests/kit/host"
)

const limen = "http://10.0.0.5:5000"

// rig is a host with daemon.json holding a log setting, and a limen serving docker.io and ghcr.io.
type rig struct {
	c        *configure.Configurer
	engine   *fake.Engine
	files    *fake.Files
	services *fake.Services
	limen    *fake.Limen
}

func newRig(files map[string]string) *rig {
	if files == nil {
		files = map[string]string{layout.Linux().DaemonJSON: `{"log-driver":"local"}`}
	}
	r := &rig{
		engine: &fake.Engine{AfterRestart: &host.Info{Mirrors: []string{limen + "/"}}},
		files:  fake.NewFiles(files),
		limen:  &fake.Limen{Served: []string{"docker.io", "ghcr.io"}},
	}
	r.services = &fake.Services{Engine: r.engine}
	r.c = &configure.Configurer{
		Layout: layout.Linux(),
		Engine: r.engine, Files: r.files, Services: r.services, Limen: r.limen.Client,
		Clock: kitclock.At(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)),
	}
	return r
}

// paths lists the paths written.
func paths(ch hostplan.Changes) []string {
	var out []string
	for _, w := range ch.Writes {
		out = append(out, w.Path)
	}
	return out
}

func TestAPlanPointsEveryRegistryAtLimen(t *testing.T) {
	r := newRig(nil)
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{layout.Linux().HostsToml("docker.io"), layout.Linux().HostsToml("ghcr.io"), layout.Linux().DaemonJSON}
	if !slices.Equal(paths(ch), want) || !ch.Restart || ch.Limen != limen || len(ch.Skipped) != 0 {
		t.Fatalf("%+v", ch)
	}
	if ch.Writes[0].Before != nil || !strings.Contains(string(ch.Writes[0].After), "[host.") {
		t.Errorf("hosts.toml %s", ch.Writes[0].After)
	}
	if string(ch.Writes[2].Before) != `{"log-driver":"local"}` || !strings.Contains(string(ch.Writes[2].After), limen) {
		t.Errorf("daemon.json %s", ch.Writes[2].After)
	}
	if len(r.files.Written) != 0 {
		t.Errorf("planning wrote %v", r.files.Written)
	}
}

func TestAConfiguredHostNeedsNothing(t *testing.T) {
	r := newRig(map[string]string{
		layout.Linux().DaemonJSON:             `{"registry-mirrors":["` + limen + `"]}`,
		layout.Linux().HostsToml("docker.io"): string(hoststoml.Render("docker.io", limen)),
		layout.Linux().HostsToml("ghcr.io"):   string(hoststoml.Render("ghcr.io", limen)),
	})
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil || len(ch.Writes) != 0 || ch.Restart {
		t.Errorf("%+v %v", ch, err)
	}
}

func TestAForeignHostsTomlIsSkippedUnlessForced(t *testing.T) {
	r := newRig(map[string]string{layout.Linux().HostsToml("ghcr.io"): "server = \"https://ghcr.io\"\n"})
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil || slices.Contains(paths(ch), layout.Linux().HostsToml("ghcr.io")) || len(ch.Skipped) != 1 || !strings.Contains(ch.Skipped[0], "--force") {
		t.Errorf("%+v %v", ch, err)
	}
	ch, err = r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen, Force: true})
	if err != nil || !slices.Contains(paths(ch), layout.Linux().HostsToml("ghcr.io")) || len(ch.Skipped) != 0 {
		t.Errorf("forced: %+v %v", ch, err)
	}
}

func TestABadLimenIsRefused(t *testing.T) {
	r := newRig(nil)
	if _, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: "10.0.0.5:5000"}); errkind.Of(err) != errkind.Invalid {
		t.Errorf("no scheme: %v", err)
	}
	r.limen.Health = errors.New("connection refused")
	if _, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen}); err == nil || !strings.Contains(err.Error(), "what it serves") {
		t.Errorf("not answering: %v", err)
	}
}

func TestApplyingBacksUpWritesAndRestarts(t *testing.T) {
	r := newRig(nil)
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.c.Apply(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	backup := layout.Linux().Backups() + "/etc_docker_daemon.json.20261029T120000Z"
	if got := string(r.files.Data[backup]); got != `{"log-driver":"local"}` {
		t.Errorf("backup %q in %v", got, r.files.Written)
	}
	if len(r.files.Written) != 4 || !strings.Contains(string(r.files.Data[layout.Linux().DaemonJSON]), limen) {
		t.Errorf("written %v", r.files.Written)
	}
	if !slices.Equal(r.services.Restarted, []string{"docker"}) {
		t.Errorf("restarted %v", r.services.Restarted)
	}
}

func TestDockerComingBackWithoutTheMirrorIsAnError(t *testing.T) {
	r := newRig(nil)
	r.engine.AfterRestart = &host.Info{}
	ch, _ := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err := r.c.Apply(context.Background(), ch); errkind.Of(err) != errkind.Upstream {
		t.Errorf("%v", err)
	}
	r = newRig(nil)
	r.services.Err = errors.New("unit docker.service not found")
	if err := r.c.Apply(context.Background(), ch); err == nil || !strings.Contains(err.Error(), "restarting docker") {
		t.Errorf("restart failing: %v", err)
	}
}

func TestWithoutARestartDockerIsLeftAlone(t *testing.T) {
	r := newRig(nil)
	ch := hostplan.Changes{Limen: limen, Writes: []hostplan.Write{{Path: layout.Linux().HostsToml("quay.io"), After: []byte("x")}}}
	if err := r.c.Apply(context.Background(), ch); err != nil || len(r.services.Restarted) != 0 || len(r.files.Written) != 1 {
		t.Errorf("%v restarted %v written %v", err, r.services.Restarted, r.files.Written)
	}
}

func TestTheContainerdStoreNeedsNoRestart(t *testing.T) {
	r := newRig(nil)
	r.engine.Current.Snapshotter = true
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil || ch.Restart || slices.Contains(paths(ch), layout.Linux().DaemonJSON) || len(ch.Writes) != 2 {
		t.Fatalf("%+v %v", ch, err)
	}
	if len(ch.Skipped) != 1 || !strings.Contains(ch.Skipped[0], "no mirror setting and no restart") {
		t.Errorf("%v", ch.Skipped)
	}
}

func TestDockerDesktopRestartsForEveryChange(t *testing.T) {
	r := newRig(map[string]string{})
	r.c.Layout = layout.Desktop("C:/Users/me")
	r.engine.Current.Snapshotter = true
	r.engine.AfterRestart = &host.Info{Snapshotter: true}
	ch, err := r.c.Plan(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil || !ch.Restart || !slices.Equal(paths(ch), []string{"C:/Users/me/.docker/certs.d/docker.io/hosts.toml", "C:/Users/me/.docker/certs.d/ghcr.io/hosts.toml"}) {
		t.Fatalf("%+v %v", ch, err)
	}
	if err := r.c.Apply(context.Background(), ch); err != nil || len(r.services.Restarted) != 1 {
		t.Fatalf("%v restarted %v", err, r.services.Restarted)
	}
	undo, err := r.c.PlanUndo(context.Background(), "")
	if err != nil || !undo.Restart || len(undo.Writes) != 2 {
		t.Errorf("%+v %v", undo, err)
	}
}
