// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror

import (
	"net/http"
	"strings"

	"github.com/nauarch/limen/internal/kernel/image"
)

// kind is what a route asks for.
type kind int

// The routes.
const (
	ping kind = iota
	manifest
	blob
)

// route is a parsed request path: what it asks for, of which registry and repository, and the
// manifest reference or blob digest.
type route struct {
	kind       kind
	registry   string
	repository string
	reference  string
}

// parseRoute reads /v2/, /v2/<name>/manifests/<reference> and /v2/<name>/blobs/<digest>. The
// name may hold slashes, so it ends at the last /manifests/ or /blobs/.
func parseRoute(r *http.Request) (route, bool) {
	p := r.URL.Path
	if p == "/v2/" || p == "/v2" {
		return route{kind: ping}, true
	}
	rest, ok := strings.CutPrefix(p, "/v2/")
	if !ok {
		return route{}, false
	}
	rt := route{registry: image.Canonical(r.URL.Query().Get("ns"))}
	m, b := strings.LastIndex(rest, "/manifests/"), strings.LastIndex(rest, "/blobs/")
	switch {
	case m > b:
		rt.kind, rt.repository, rt.reference = manifest, rest[:m], rest[m+len("/manifests/"):]
	case b > m:
		rt.kind, rt.repository, rt.reference = blob, rest[:b], rest[b+len("/blobs/"):]
	default:
		return route{}, false
	}
	if rt.repository == "" || rt.reference == "" || strings.Contains(rt.reference, "/") {
		return route{}, false
	}
	return rt, true
}
