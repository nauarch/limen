// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package mirror_test

// Verifies:
//   - /v2/ answers 200 with the registry API version header, so clients take limen for a registry
//   - Anything but GET and HEAD is 405, an unknown path 404
//   - A manifest is served with its digest, media type and length; a HEAD without the body
//   - The ns parameter picks the registry, Docker Hub without one; the client's Accept is passed on
//   - Failures map to statuses a client falls back on: 404, 403, 400, 502, their messages saying
//     they are limen's and that the registry is tried next

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/frontdoor/mirror"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// puller is a scripted pull.Puller that records what it was asked.
type puller struct {
	err        error
	manifest   pull.Manifest
	blob       []byte
	seekable   bool
	registry   string
	repository string
	accept     []string
}

// Serves serves everything.
func (p *puller) Serves(string) bool { return true }

// Manifest answers the scripted manifest or failure.
func (p *puller) Manifest(_ context.Context, registry, repository, _ string, accept []string) (pull.Manifest, error) {
	p.registry, p.repository, p.accept = registry, repository, accept
	return p.manifest, p.err
}

// Blob answers the scripted blob, seekable or streaming, or the failure.
func (p *puller) Blob(_ context.Context, registry, repository string, _ digest.Digest) (pull.Blob, error) {
	p.registry, p.repository = registry, repository
	if p.err != nil {
		return pull.Blob{}, p.err
	}
	if p.seekable {
		return pull.Blob{Size: int64(len(p.blob)), Body: nopSeekCloser{bytes.NewReader(p.blob)}}, nil
	}
	return pull.Blob{Size: int64(len(p.blob)), Body: io.NopCloser(bytes.NewReader(p.blob))}, nil
}

// nopSeekCloser is a seekable body with nothing to close.
type nopSeekCloser struct{ *bytes.Reader }

// Close does nothing.
func (nopSeekCloser) Close() error { return nil }

// do sends one request to a door over p.
func do(p *puller, method, target string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	mirror.New(mirror.Options{Puller: p}).ServeHTTP(w, req)
	return w
}

func TestTheDoorAnswersAsARegistry(t *testing.T) {
	w := do(&puller{}, http.MethodGet, "/v2/", nil)
	if w.Code != http.StatusOK || w.Header().Get("Docker-Distribution-API-Version") != "registry/2.0" {
		t.Errorf("ping: %d %v", w.Code, w.Header())
	}
	if w := do(&puller{}, http.MethodPut, "/v2/a/manifests/latest", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT: %d", w.Code)
	}
	for _, path := range []string{"/v2/a/tags/list", "/other", "/v2/a/manifests/", "/v2//blobs/x"} {
		if w := do(&puller{}, http.MethodGet, path, nil); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}

func TestAManifestIsServedWithItsDigest(t *testing.T) {
	body := []byte(`{"schemaVersion":2}`)
	p := &puller{manifest: pull.Manifest{MediaType: "application/vnd.oci.image.index.v1+json", Digest: digest.Of(body), Body: body}}
	header := http.Header{"Accept": {"application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json"}}
	w := do(p, http.MethodGet, "/v2/vllm/vllm-openai/manifests/v0.32.0", header)
	if w.Code != http.StatusOK || w.Body.String() != string(body) {
		t.Fatalf("%d %q", w.Code, w.Body)
	}
	if w.Header().Get("Docker-Content-Digest") != digest.Of(body).String() || w.Header().Get("Content-Type") != p.manifest.MediaType {
		t.Errorf("headers %v", w.Header())
	}
	if p.registry != "docker.io" || p.repository != "vllm/vllm-openai" || len(p.accept) != 2 {
		t.Errorf("asked %s %s %v", p.registry, p.repository, p.accept)
	}
	head := do(p, http.MethodHead, "/v2/vllm/vllm-openai/manifests/v0.32.0?ns=ghcr.io", nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != fmt.Sprint(len(body)) {
		t.Errorf("HEAD: %d, %d body bytes, %v", head.Code, head.Body.Len(), head.Header())
	}
	if p.registry != "ghcr.io" {
		t.Errorf("ns picked %s", p.registry)
	}
}

func TestFailuresMapToStatusesClientsFallBackOn(t *testing.T) {
	for err, want := range map[error]int{
		errkind.ErrNotFound:    http.StatusNotFound,
		errkind.ErrForbidden:   http.StatusForbidden,
		errkind.ErrInvalid:     http.StatusBadRequest,
		errkind.ErrUnreachable: http.StatusBadGateway,
		errkind.ErrUpstream:    http.StatusBadGateway,
	} {
		w := do(&puller{err: err}, http.MethodGet, "/v2/library/nginx/manifests/latest", nil)
		if w.Code != want || !strings.Contains(w.Body.String(), `"errors"`) || !strings.Contains(w.Body.String(), "limen: ") || !strings.Contains(w.Body.String(), "tries the registry itself next") {
			t.Errorf("%v: %d %s", err, w.Code, w.Body)
		}
	}
}
