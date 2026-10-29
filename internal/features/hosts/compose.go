// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hosts

import (
	"context"
	"fmt"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/app/configure"
	"github.com/nauarch/limen/internal/features/hosts/app/migrate"
	"github.com/nauarch/limen/internal/features/hosts/app/report"
	"github.com/nauarch/limen/internal/features/hosts/app/verify"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Options are the host limenctl works on: where it keeps Docker's files, its engine, files,
// network and services, Docker compose and the host's scripts, how to reach a limen by URL, and
// the clock.
type Options struct {
	Layout   layout.Layout
	Engine   host.Engine
	Files    host.Files
	Network  host.Network
	Services host.Services
	Compose  host.Compose
	Scripts  host.Scripts
	Limen    func(url string) host.Limen
	Clock    clock.Clock
}

// Hosts reports on a Docker host, points it at a limen and moves it between image stores: a
// hostplan.Planner and a hostplan.Migrator.
type Hosts struct {
	*migrate.Migrator
	gatherer   *report.Gatherer
	configurer *configure.Configurer
	verifier   *verify.Verifier
}

var (
	_ hostplan.Planner  = (*Hosts)(nil)
	_ hostplan.Migrator = (*Hosts)(nil)
)

// Compose builds the feature; everything but the clock is required.
func Compose(o Options) (*Hosts, error) {
	if o.Layout.CertsDir == "" || o.Layout.DaemonJSON == "" || o.Layout.StateDir == "" || o.Engine == nil || o.Files == nil || o.Network == nil || o.Services == nil || o.Compose == nil || o.Scripts == nil || o.Limen == nil {
		return nil, fmt.Errorf("hosts: a layout, an engine, files, network, services, compose, scripts and a limen client are required: %w", errkind.ErrInvalid)
	}
	if o.Clock == nil {
		o.Clock = clock.System{}
	}
	g := &report.Gatherer{Layout: o.Layout, Engine: o.Engine, Files: o.Files, Network: o.Network, Compose: o.Compose, Limen: o.Limen}
	c := &configure.Configurer{Layout: o.Layout, Engine: o.Engine, Files: o.Files, Services: o.Services, Limen: o.Limen, Clock: o.Clock}
	return &Hosts{
		Migrator: &migrate.Migrator{
			Gatherer: g, Configurer: c, Engine: o.Engine, Files: o.Files, Services: o.Services,
			Compose: o.Compose, Scripts: o.Scripts, Limen: o.Limen, Clock: o.Clock,
		},
		gatherer:   g,
		configurer: c,
		verifier:   &verify.Verifier{Engine: o.Engine, Limen: o.Limen},
	}, nil
}

// Report reads the host and decides what it says.
func (h *Hosts) Report(ctx context.Context, limen string) (hostplan.Report, error) {
	f, err := h.gatherer.Gather(ctx, limen)
	if err != nil {
		return hostplan.Report{}, err
	}
	return plan.Decide(f), nil
}

// Configure works out the changes pointing the host at a limen.
func (h *Hosts) Configure(ctx context.Context, r hostplan.ConfigureRequest) (hostplan.Changes, error) {
	return h.configurer.Plan(ctx, r)
}

// Apply makes the changes.
func (h *Hosts) Apply(ctx context.Context, c hostplan.Changes) error {
	return h.configurer.Apply(ctx, c)
}

// Unconfigure works out the changes taking limenctl's configuration off the host.
func (h *Hosts) Unconfigure(ctx context.Context, limen string) (hostplan.Changes, error) {
	return h.configurer.PlanUndo(ctx, limen)
}

// Layout is where a host keeps Docker's files and limenctl its own.
type Layout = layout.Layout

// LinuxLayout is a Linux host's layout, its files read from /.
func LinuxLayout() Layout { return layout.Linux() }

// DesktopLayout is Docker Desktop's layout in the user's home, given with forward slashes.
func DesktopLayout(home string) Layout { return layout.Desktop(home) }

// Verify pulls a small image and asks the limen whether the pull reached it.
func (h *Hosts) Verify(ctx context.Context, limen string) (hostplan.Verification, error) {
	return h.verifier.Verify(ctx, limen)
}
