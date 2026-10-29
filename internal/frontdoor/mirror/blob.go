// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nauarch/limen/internal/kernel/digest"
)

// blob serves a blob. A blob complete on disk is served with range support; one still arriving
// is streamed as it comes. A HEAD of a blob not yet cached starts its download all the same, so
// the GET that follows finds it under way.
func (d *door) blob(w http.ResponseWriter, r *http.Request, rt route) {
	dg, err := digest.Parse(rt.reference)
	if err != nil {
		fail(w, r, d.log, err)
		return
	}
	got, err := d.puller.Blob(r.Context(), rt.registry, rt.repository, dg)
	if err != nil {
		fail(w, r, d.log, err)
		return
	}
	defer got.Body.Close()
	h := w.Header()
	h.Set("Content-Type", "application/octet-stream")
	h.Set("Docker-Content-Digest", dg.String())
	h.Set("ETag", `"`+dg.String()+`"`)
	if seeker, ok := got.Body.(io.ReadSeeker); ok {
		http.ServeContent(w, r, "", time.Time{}, seeker)
		return
	}
	if got.Size >= 0 {
		h.Set("Content-Length", strconv.FormatInt(got.Size, 10))
	}
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, got.Body); err != nil {
		d.log.Info("blob stream ended early", "digest", dg, "err", err)
	}
}
