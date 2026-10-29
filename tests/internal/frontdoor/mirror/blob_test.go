// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror_test

// Verifies:
//   - A blob complete on disk answers range requests
//   - A blob still arriving is streamed whole, with its length when known
//   - A malformed digest is a bad request

import (
	"net/http"
	"testing"

	"github.com/nauarch/limen/internal/kernel/digest"
)

func TestABlobOnDiskAnswersRanges(t *testing.T) {
	data := []byte("0123456789")
	p := &puller{blob: data, seekable: true}
	w := do(p, http.MethodGet, "/v2/library/nginx/blobs/"+digest.Of(data).String(), http.Header{"Range": {"bytes=4-"}})
	if w.Code != http.StatusPartialContent || w.Body.String() != "456789" {
		t.Fatalf("%d %q", w.Code, w.Body)
	}
	if w.Header().Get("Docker-Content-Digest") != digest.Of(data).String() {
		t.Errorf("headers %v", w.Header())
	}
}

func TestABlobStillArrivingIsStreamed(t *testing.T) {
	data := []byte("streamed")
	w := do(&puller{blob: data}, http.MethodGet, "/v2/library/nginx/blobs/"+digest.Of(data).String(), nil)
	if w.Code != http.StatusOK || w.Body.String() != "streamed" || w.Header().Get("Content-Length") != "8" {
		t.Fatalf("%d %q %v", w.Code, w.Body, w.Header())
	}
}

func TestAMalformedDigestIsABadRequest(t *testing.T) {
	if w := do(&puller{}, http.MethodGet, "/v2/library/nginx/blobs/sha256:nothex", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("%d", w.Code)
	}
}
