// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror

import (
	"log/slog"
	"net/http"

	"github.com/nauarch/limen/internal/contracts/pull"
)

// Options are what the door serves (any pull.Puller) and where it logs.
type Options struct {
	Puller pull.Puller
	Log    *slog.Logger
}

// door serves the registry API's pull half, as a mirror: GET and HEAD of /v2/, of
// /v2/<name>/manifests/<reference> and of /v2/<name>/blobs/<digest>. The registry is named by
// the ns query parameter containerd adds for mirrors, and is Docker Hub without one (as Docker's
// registry-mirrors setting asks). Everything else is 404 or 405, so a client falls back to the
// real registry.
type door struct {
	puller pull.Puller
	log    *slog.Logger
}

// New builds the door.
func New(o Options) http.Handler {
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &door{puller: o.Puller, log: log}
}

// ServeHTTP routes a request.
func (d *door) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		answer(w, r, d.log, http.StatusMethodNotAllowed, "UNSUPPORTED", "a mirror only serves pulls")
		return
	}
	rt, ok := parseRoute(r)
	if !ok {
		answer(w, r, d.log, http.StatusNotFound, "NAME_UNKNOWN", "no such route")
		return
	}
	switch rt.kind {
	case ping:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	case manifest:
		d.manifest(w, r, rt)
	case blob:
		d.blob(w, r, rt)
	}
}
