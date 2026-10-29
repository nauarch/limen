// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// statusOf is the HTTP status and registry error code a failure is answered with. Every one of
// them makes a client fall back to the real registry.
func statusOf(err error) (int, string) {
	switch errkind.Of(err) {
	case errkind.NotFound:
		return http.StatusNotFound, "MANIFEST_UNKNOWN"
	case errkind.Invalid:
		return http.StatusBadRequest, "NAME_INVALID"
	case errkind.Forbidden:
		return http.StatusForbidden, "DENIED"
	case errkind.Upstream, errkind.Unreachable:
		return http.StatusBadGateway, "UNAVAILABLE"
	case errkind.Canceled:
		return 499, "CANCELED"
	default:
		return http.StatusInternalServerError, "UNKNOWN"
	}
}

// fail answers a failure, logging what is limen's or upstream's fault. The message says it
// comes from limen and that the client goes on to the registry, since a client whose fallback
// fails too reports the first error it met, which is limen's.
func fail(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	status, code := statusOf(err)
	if status >= http.StatusInternalServerError {
		log.Warn("pull failed", "path", r.URL.Path, "ns", r.URL.Query().Get("ns"), "err", err)
	}
	answer(w, r, log, status, code, "limen: "+err.Error()+" (the client tries the registry itself next)")
}

// answer writes the registry API's error body.
func answer(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	body := map[string]any{"errors": []map[string]string{{"code": code, "message": message}}}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Debug("error answer not written", "err", err)
	}
}
