// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ops

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Options are what the door serves: the health and counters to report, the prefetcher to start
// jobs on, the version limen was built as, and where it logs.
type Options struct {
	Reporter   ops.Reporter
	Prefetcher prefetch.Prefetcher
	Version    string
	Log        *slog.Logger
}

// door serves limen's operations: GET /healthz (200, or 503 when limen cannot serve), GET
// /metrics (Prometheus text), GET /api/registries (what is served), POST /api/prefetch (starts a
// job, 202) and GET /api/prefetch/{id} (a job's progress).
type door struct {
	o Options
}

// New builds the door.
func New(o Options) http.Handler {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	d := &door{o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", d.healthz)
	mux.HandleFunc("GET /metrics", d.metrics)
	mux.HandleFunc("GET /api/registries", d.registries)
	mux.HandleFunc("POST /api/prefetch", d.startPrefetch)
	mux.HandleFunc("GET /api/prefetch/{id}", d.prefetchJob)
	return mux
}

// healthz answers whether limen can serve.
func (d *door) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := d.o.Reporter.Healthy(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(err.Error() + "\n"))
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

// registries answers the registries limen serves, as {"registries": [...]}.
func (d *door) registries(w http.ResponseWriter, _ *http.Request) {
	d.reply(w, http.StatusOK, map[string][]string{"registries": d.o.Reporter.Registries()})
}

// statusOf is the HTTP status a failure is answered with.
func statusOf(err error) int {
	switch errkind.Of(err) {
	case errkind.NotFound:
		return http.StatusNotFound
	case errkind.Invalid:
		return http.StatusBadRequest
	case errkind.Forbidden:
		return http.StatusForbidden
	case errkind.Conflict:
		return http.StatusConflict
	case errkind.Upstream, errkind.Unreachable:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// reply writes v as JSON with status.
func (d *door) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		d.o.Log.Debug("answer not written", "err", err)
	}
}

// fail answers a failure as {"error": "..."}.
func (d *door) fail(w http.ResponseWriter, err error) {
	d.reply(w, statusOf(err), map[string]string{"error": err.Error()})
}
