// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli_test

// Verifies:
//   - No command or an unknown one prints the help and exits 2; help exits 0
//   - plan prints the report's sections and findings, checked against the --limen given
//   - configure needs --limen; without --yes it prints the changes and makes none
//   - With --yes it makes them; a host already configured needs nothing
//   - unconfigure prints what it removes, and removes only with --yes
//   - A failing planner exits 1 with the error
//   - verify needs --limen, and exits 1 when the pull went around limen

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/frontdoor/cli"
)

// planner answers a scripted report and changes, recording what it was asked.
type planner struct {
	report    hostplan.Report
	changes   hostplan.Changes
	undo      hostplan.Changes
	undoLimen string
	err       error
	limen     string
	request   hostplan.ConfigureRequest
	applied   bool
	verified  hostplan.Verification
}

func (p *planner) Report(_ context.Context, limen string) (hostplan.Report, error) {
	p.limen = limen
	return p.report, p.err
}

func (p *planner) Configure(_ context.Context, r hostplan.ConfigureRequest) (hostplan.Changes, error) {
	p.request = r
	return p.changes, p.err
}

func (p *planner) Unconfigure(_ context.Context, limen string) (hostplan.Changes, error) {
	p.undoLimen = limen
	return p.undo, p.err
}

func (p *planner) Apply(context.Context, hostplan.Changes) error {
	p.applied = true
	return p.err
}

func (p *planner) Verify(_ context.Context, limen string) (hostplan.Verification, error) {
	p.limen = limen
	return p.verified, p.err
}

// run runs a command line against p, giving the exit code and what was printed.
func run(p *planner, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Options{Planner: p, Out: &out, Err: &errOut})
	return code, out.String(), errOut.String()
}

// changes points docker.io at limen and changes daemon.json.
func changes() hostplan.Changes {
	return hostplan.Changes{
		Limen: "http://10.0.0.5:5000",
		Writes: []hostplan.Write{
			{Path: "/etc/docker/certs.d/docker.io/hosts.toml", After: []byte("server = \"https://registry-1.docker.io\"\n")},
			{Path: "/etc/docker/daemon.json", Before: []byte("{}\n"), After: []byte("{\"registry-mirrors\":[\"http://10.0.0.5:5000\"]}\n")},
		},
		Skipped: []string{"/etc/docker/certs.d/ghcr.io/hosts.toml: not written by limenctl"},
		Restart: true,
	}
}

func TestCommandsAreChosen(t *testing.T) {
	if code, _, errOut := run(&planner{}); code != 2 || !strings.Contains(errOut, "Commands:") {
		t.Errorf("none: %d %s", code, errOut)
	}
	if code, _, errOut := run(&planner{}, "frobnicate"); code != 2 || !strings.Contains(errOut, `unknown command "frobnicate"`) {
		t.Errorf("unknown: %d %s", code, errOut)
	}
	if code, out, _ := run(&planner{}, "help"); code != 0 || !strings.Contains(out, "configure") {
		t.Errorf("help: %d %s", code, out)
	}
	if code, _, _ := run(&planner{}, "plan", "--nonsense"); code != 2 {
		t.Errorf("bad flag: %d", code)
	}
}

func TestPlanPrintsTheReport(t *testing.T) {
	p := &planner{report: hostplan.Report{
		Version: "29.9.2", Store: "classic",
		Projects: []hostplan.Project{{Name: "dns", Dir: "/srv/dns", Running: 1, Total: 1, Scripts: []string{"start.sh"}}},
		Loose:    []string{"oneoff"},
		InUse:    hostplan.Images{Count: 2, Bytes: 3 << 30},
		Disk:     []hostplan.Disk{{Path: "/var/lib/containerd", Free: -1}},
		Mirror:   hostplan.Mirror{Limen: "http://10.0.0.5:5000", Registries: []string{"docker.io"}},
		Findings: []hostplan.Finding{{Level: hostplan.Block, Text: "swarm mode is on"}},
	}}
	code, out, _ := run(p, "plan", "--limen", "http://10.0.0.5:5000")
	if code != 0 || p.limen != "http://10.0.0.5:5000" {
		t.Fatalf("%d %q", code, p.limen)
	}
	for _, want := range []string{
		"Docker 29.9.2, classic image store", "dns", "scripts: start.sh", "not from compose: oneoff",
		"in use", "3GiB", "free space unknown", "serves:      docker.io", "hosts.toml:  (none)", "block swarm mode is on",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestConfigureNeedsALimen(t *testing.T) {
	if code, _, errOut := run(&planner{}, "configure"); code != 2 || !strings.Contains(errOut, "needs --limen") {
		t.Errorf("%d %s", code, errOut)
	}
}

func TestConfigureIsADryRunWithoutYes(t *testing.T) {
	p := &planner{changes: changes()}
	code, out, _ := run(p, "configure", "--limen", "http://10.0.0.5:5000", "--force")
	if code != 0 || p.applied {
		t.Fatalf("%d applied %v", code, p.applied)
	}
	if p.request != (hostplan.ConfigureRequest{Limen: "http://10.0.0.5:5000", Force: true}) {
		t.Errorf("asked %+v", p.request)
	}
	for _, want := range []string{
		"docker.io/hosts.toml (new)", "+ server = ", "daemon.json (changed, the old one is backed up)", "- {}",
		"left alone: /etc/docker/certs.d/ghcr.io", "Docker restarts", "Dry run: nothing was changed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestConfigureWithYesMakesTheChanges(t *testing.T) {
	p := &planner{changes: changes()}
	if code, out, _ := run(p, "configure", "--limen", "http://10.0.0.5:5000", "--yes"); code != 0 || !p.applied || !strings.Contains(out, "Done in") {
		t.Errorf("%d applied %v\n%s", code, p.applied, out)
	}
	p = &planner{changes: hostplan.Changes{Limen: "http://10.0.0.5:5000"}}
	if code, out, _ := run(p, "configure", "--limen", "http://10.0.0.5:5000", "--yes"); code != 0 || p.applied || !strings.Contains(out, "nothing to change") {
		t.Errorf("configured: %d applied %v\n%s", code, p.applied, out)
	}
}

func TestAFailingPlannerExitsOne(t *testing.T) {
	p := &planner{err: errors.New("cannot connect to the Docker daemon")}
	if code, _, errOut := run(p, "plan"); code != 1 || !strings.Contains(errOut, "limenctl: cannot connect") {
		t.Errorf("%d %s", code, errOut)
	}
}

func TestNewHostsTomlFilesAreListedTogether(t *testing.T) {
	ch := changes()
	ch.Writes = append(ch.Writes, hostplan.Write{Path: "/etc/docker/certs.d/ghcr.io/hosts.toml", After: []byte("server = \"https://ghcr.io\"\n")})
	_, out, _ := run(&planner{changes: ch}, "configure", "--limen", "http://10.0.0.5:5000")
	if !strings.Contains(out, "hosts.toml for 2 registries (new): docker.io, ghcr.io") || strings.Count(out, "+ server = ") != 1 || !strings.Contains(out, "daemon.json (changed") {
		t.Errorf("\n%s", out)
	}
}

func TestUnconfigureIsADryRunWithoutYes(t *testing.T) {
	undo := hostplan.Changes{Writes: []hostplan.Write{{Path: "/etc/docker/certs.d/docker.io/hosts.toml", Before: []byte("server = \"https://registry-1.docker.io\"\n")}}}
	p := &planner{undo: undo}
	code, out, _ := run(p, "unconfigure")
	if code != 0 || p.applied || !strings.Contains(out, "docker.io/hosts.toml (removed, a backup is kept)") || !strings.Contains(out, "Dry run") {
		t.Errorf("%d %v\n%s", code, p.applied, out)
	}
	if code, _, _ := run(p, "unconfigure", "--limen", "http://192.0.2.10:5000", "--yes"); code != 0 || !p.applied || p.undoLimen != "http://192.0.2.10:5000" {
		t.Errorf("--yes: %d %v %q", code, p.applied, p.undoLimen)
	}
	if code, out, _ := run(&planner{}, "unconfigure", "--yes"); code != 0 || !strings.Contains(out, "nothing to undo") {
		t.Errorf("nothing: %d\n%s", code, out)
	}
}

func TestVerifySaysWhetherThePullReachedLimen(t *testing.T) {
	if code, _, _ := run(&planner{}, "verify"); code != 2 {
		t.Errorf("without --limen: %d", code)
	}
	p := &planner{verified: hostplan.Verification{Image: "docker.io/library/hello-world:latest", Requests: 3, Removed: true}}
	code, out, _ := run(p, "verify", "--limen", "http://192.0.2.10:5000")
	if code != 0 || p.limen != "http://192.0.2.10:5000" || !strings.Contains(out, "counted 3 requests") || !strings.Contains(out, "removed it again") {
		t.Errorf("through: %d %s", code, out)
	}
	p.verified.Requests = 0
	if code, out, errOut := run(p, "verify", "--limen", "http://192.0.2.10:5000"); code != 1 || !strings.Contains(out, "went around it") || !strings.Contains(errOut, "went around limen") {
		t.Errorf("around: %d %s %s", code, out, errOut)
	}
}
