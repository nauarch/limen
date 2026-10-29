// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ops_test

// Verifies:
//   - A prefetch request starts a job: 202, the job, and where to follow it
//   - A request without images, with unknown fields or not JSON is a bad request; a refused one
//     keeps its kind (403 for a registry not served)
//   - A job is followed by its ID; an unknown one is 404

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestAPrefetchRequestStartsAJob(t *testing.T) {
	f := &fake{job: prefetch.Job{ID: "abc", State: prefetch.Running, Images: 1}}
	w := do(f, http.MethodPost, "/api/prefetch", `{"images":["nginx:1.27"],"platforms":["linux/amd64"]}`)
	if w.Code != http.StatusAccepted || w.Header().Get("Location") != "/api/prefetch/abc" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	var got prefetch.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.ID != "abc" {
		t.Fatalf("%q %v", w.Body, err)
	}
	if len(f.got.Images) != 1 || f.got.Platforms[0] != "linux/amd64" {
		t.Errorf("asked %+v", f.got)
	}
	if w := do(f, http.MethodGet, "/api/prefetch/abc", ""); w.Code != http.StatusOK {
		t.Errorf("following: %d", w.Code)
	}
	if w := do(f, http.MethodGet, "/api/prefetch/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown job: %d", w.Code)
	}
}

func TestABadPrefetchRequestIsRefused(t *testing.T) {
	for _, body := range []string{`{"images":[]}`, `{"images":["a"],"extra":1}`, `not json`} {
		if w := do(&fake{}, http.MethodPost, "/api/prefetch", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	if w := do(&fake{err: errkind.ErrForbidden}, http.MethodPost, "/api/prefetch", `{"images":["quay.io/a/b"]}`); w.Code != http.StatusForbidden {
		t.Errorf("refused: %d", w.Code)
	}
}
