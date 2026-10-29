// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hosts_test

// Verifies:
//   - The feature needs an engine, files, network, services, compose, scripts and a limen client
//   - Composed, it reports on a host, plans pointing it at limen, and applies the plan

import (
	"context"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	"github.com/nauarch/limen/internal/kernel/errkind"
	fake "github.com/nauarch/limen/tests/kit/host"
)

func TestTheFeatureNeedsItsPorts(t *testing.T) {
	if _, err := hosts.Compose(hosts.Options{}); errkind.Of(err) != errkind.Invalid {
		t.Errorf("%v", err)
	}
}

func TestTheFeatureReportsPlansAndApplies(t *testing.T) {
	const limen = "http://10.0.0.5:5000"
	e := &fake.Engine{Current: host.Info{Version: "29.9.2"}, AfterRestart: &host.Info{Mirrors: []string{limen}}}
	files := fake.NewFiles(nil)
	l := &fake.Limen{Served: []string{"docker.io"}}
	h, err := hosts.Compose(hosts.Options{Layout: layout.Linux(), Engine: e, Files: files, Network: fake.Network{}, Services: &fake.Services{Engine: e}, Compose: &fake.Compose{}, Scripts: &fake.Scripts{}, Limen: l.Client})
	if err != nil {
		t.Fatal(err)
	}
	r, err := h.Report(context.Background(), limen)
	if err != nil || r.Version != "29.9.2" || len(r.Mirror.Registries) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	ch, err := h.Configure(context.Background(), hostplan.ConfigureRequest{Limen: limen})
	if err != nil || len(ch.Writes) != 2 {
		t.Fatalf("%+v %v", ch, err)
	}
	if err := h.Apply(context.Background(), ch); err != nil || len(files.Written) != 2 {
		t.Errorf("%v written %v", err, files.Written)
	}
}
