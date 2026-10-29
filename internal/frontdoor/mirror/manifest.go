// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror

import (
	"net/http"
	"strconv"
	"strings"
)

// manifest serves a manifest by tag or digest, with the digest and media type a client checks.
func (d *door) manifest(w http.ResponseWriter, r *http.Request, rt route) {
	got, err := d.puller.Manifest(r.Context(), rt.registry, rt.repository, rt.reference, accepted(r))
	if err != nil {
		fail(w, r, d.log, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", got.MediaType)
	h.Set("Docker-Content-Digest", got.Digest.String())
	h.Set("Content-Length", strconv.Itoa(len(got.Body)))
	h.Set("ETag", `"`+got.Digest.String()+`"`)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(got.Body)
}

// accepted lists the media types a request accepts, from every Accept header.
func accepted(r *http.Request) []string {
	var out []string
	for _, h := range r.Header.Values("Accept") {
		for part := range strings.SplitSeq(h, ",") {
			if t := strings.TrimSpace(part); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}
