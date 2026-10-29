// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ops

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// maxRequest is the largest prefetch request read.
const maxRequest = 1 << 20

// startPrefetch starts a job from a JSON prefetch.Request and answers 202 with the job.
func (d *door) startPrefetch(w http.ResponseWriter, r *http.Request) {
	var req prefetch.Request
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		d.fail(w, fmt.Errorf("prefetch request: %w: %w", errkind.ErrInvalid, err))
		return
	}
	if len(req.Images) == 0 {
		d.fail(w, fmt.Errorf("prefetch request names no image: %w", errkind.ErrInvalid))
		return
	}
	job, err := d.o.Prefetcher.Start(r.Context(), req)
	if err != nil {
		d.fail(w, err)
		return
	}
	w.Header().Set("Location", "/api/prefetch/"+job.ID)
	d.reply(w, http.StatusAccepted, job)
}

// prefetchJob answers a job's progress.
func (d *door) prefetchJob(w http.ResponseWriter, r *http.Request) {
	job, err := d.o.Prefetcher.Job(r.PathValue("id"))
	if err != nil {
		d.fail(w, err)
		return
	}
	d.reply(w, http.StatusOK, job)
}
