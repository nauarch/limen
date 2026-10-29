// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ops_test

// Verifies:
//   - /healthz answers 200 when limen can serve and 503 with the reason when it cannot
//   - The registries limen serves are listed
//   - Other paths and methods are not served

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	opsdoor "github.com/nauarch/limen/internal/frontdoor/ops"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// fake is a scripted reporter and prefetcher that records the request it was given.
type fake struct {
	health error
	stats  ops.Stats
	job    prefetch.Job
	err    error
	got    prefetch.Request
}

// Healthy answers the scripted health.
func (f *fake) Healthy() error { return f.health }

// Stats answers the scripted counters.
func (f *fake) Stats() ops.Stats { return f.stats }

// Registries answers two registries.
func (f *fake) Registries() []string { return []string{"docker.io", "ghcr.io"} }

// Start records the request and answers the scripted job or failure.
func (f *fake) Start(_ context.Context, r prefetch.Request) (prefetch.Job, error) {
	f.got = r
	return f.job, f.err
}

// Job answers the scripted job when the ID matches.
func (f *fake) Job(id string) (prefetch.Job, error) {
	if id != f.job.ID {
		return prefetch.Job{}, errkind.ErrNotFound
	}
	return f.job, nil
}

// do sends one request to a door over f.
func do(f *fake, method, target, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	opsdoor.New(opsdoor.Options{Reporter: f, Prefetcher: f, Version: `v1.2.3"x`}).ServeHTTP(w, httptest.NewRequest(method, target, strings.NewReader(body)))
	return w
}

func TestHealthzTellsWhetherLimenCanServe(t *testing.T) {
	if w := do(&fake{}, http.MethodGet, "/healthz", ""); w.Code != http.StatusOK || w.Body.String() != "ok\n" {
		t.Errorf("healthy: %d %q", w.Code, w.Body)
	}
	w := do(&fake{health: errors.New("marker unreadable")}, http.MethodGet, "/healthz", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "marker unreadable") {
		t.Errorf("unhealthy: %d %q", w.Code, w.Body)
	}
}

func TestOtherPathsAreNotServed(t *testing.T) {
	for _, c := range []struct{ method, path string }{{http.MethodGet, "/"}, {http.MethodPost, "/healthz"}, {http.MethodGet, "/api/prefetch"}} {
		if w := do(&fake{}, c.method, c.path, ""); w.Code < 400 {
			t.Errorf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
}

func TestTheServedRegistriesAreListed(t *testing.T) {
	w := do(&fake{}, http.MethodGet, "/api/registries", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"registries":["docker.io","ghcr.io"]}` {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
