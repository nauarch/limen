// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"

	"github.com/nauarch/limen/internal/kernel/digest"
)

// The things a Registry counts, as Count takes them.
const (
	HeadManifest = "HEAD manifest"
	GetManifest  = "GET manifest"
	GetBlob      = "GET blob"
	Storage      = "GET storage"
	Token        = "GET token"
)

// token is the one pull token the registry hands out.
const token = "kit-token"

// Registry is an in-process registry for tests. It serves the manifests and blobs a test puts in,
// as its Options shape it, counts what it is asked, can hold blob bodies halfway, and can stall
// answering altogether, each until released.
type Registry struct {
	server *httptest.Server

	opts Options

	mu        sync.Mutex
	manifests map[string]manifest
	blobs     map[digest.Digest][]byte
	counts    map[string]int
	held      chan struct{}
	stalled   chan struct{}
}

// manifest is a stored manifest.
type manifest struct {
	mediaType string
	body      []byte
}

// Options shape a registry: RequireToken answers 401 with a Bearer challenge until a request
// carries the pull token; RedirectBlobs answers blob GETs with a redirect to a storage path.
type Options struct {
	RequireToken  bool
	RedirectBlobs bool
}

// New starts a registry; Close stops it.
func New(o Options) *Registry {
	r := &Registry{opts: o, manifests: map[string]manifest{}, blobs: map[digest.Digest][]byte{}, counts: map[string]int{}}
	r.server = httptest.NewServer(http.HandlerFunc(r.serve))
	return r
}

// URL is the registry's base URL.
func (r *Registry) URL() string { return r.server.URL }

// Close stops the registry; requests to it then fail to connect.
func (r *Registry) Close() { r.server.Close() }

// PutManifest serves a manifest under a repository's tag (when tag is not empty) and its digest.
func (r *Registry) PutManifest(repository, tag, mediaType string, body []byte) digest.Digest {
	d := digest.Of(body)
	r.mu.Lock()
	defer r.mu.Unlock()
	m := manifest{mediaType: mediaType, body: body}
	r.manifests[repository+"@"+d.String()] = m
	if tag != "" {
		r.manifests[repository+":"+tag] = m
	}
	return d
}

// DeleteTag stops serving a tag.
func (r *Registry) DeleteTag(repository, tag string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.manifests, repository+":"+tag)
}

// PutBlob serves a blob under its digest.
func (r *Registry) PutBlob(data []byte) digest.Digest {
	d := digest.Of(data)
	r.PutBlobAs(d, data)
	return d
}

// PutBlobAs serves data under d, which need not be its digest: a corrupt blob.
func (r *Registry) PutBlobAs(d digest.Digest, data []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blobs[d] = data
}

// Hold makes blob bodies stop halfway until release is called.
func (r *Registry) Hold() (release func()) {
	ch := make(chan struct{})
	r.mu.Lock()
	r.held = ch
	r.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

// Stall makes every registry API request wait, before answering anything, until release is called.
func (r *Registry) Stall() (release func()) {
	ch := make(chan struct{})
	r.mu.Lock()
	r.stalled = ch
	r.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

// Count is how often the registry was asked one of the counted things.
func (r *Registry) Count(what string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[what]
}

// count notes one request.
func (r *Registry) count(what string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[what]++
}

// serve routes a request.
func (r *Registry) serve(w http.ResponseWriter, req *http.Request) {
	p := req.URL.Path
	switch {
	case p == "/token":
		r.count(Token)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"token": token, "expires_in": 300})
	case strings.HasPrefix(p, "/storage/"):
		r.count(Storage)
		r.sendBlob(w, digest.Digest(strings.TrimPrefix(p, "/storage/")))
	case strings.HasPrefix(p, "/v2/"):
		r.mu.Lock()
		stalled := r.stalled
		r.mu.Unlock()
		if stalled != nil {
			<-stalled
		}
		if !r.signedIn(w, req) {
			return
		}
		rest := strings.TrimPrefix(p, "/v2/")
		if i := strings.LastIndex(rest, "/manifests/"); i >= 0 {
			r.serveManifest(w, req, rest[:i], rest[i+len("/manifests/"):])
			return
		}
		if i := strings.LastIndex(rest, "/blobs/"); i >= 0 {
			r.serveBlob(w, req, digest.Digest(rest[i+len("/blobs/"):]))
			return
		}
		http.NotFound(w, req)
	default:
		http.NotFound(w, req)
	}
}

// signedIn answers 401 with a Bearer challenge to a request without the token, when one is needed.
func (r *Registry) signedIn(w http.ResponseWriter, req *http.Request) bool {
	if !r.opts.RequireToken || req.Header.Get("Authorization") == "Bearer "+token {
		return true
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="kit"`, r.server.URL))
	w.WriteHeader(http.StatusUnauthorized)
	return false
}

// serveManifest answers a manifest HEAD or GET by tag or digest.
func (r *Registry) serveManifest(w http.ResponseWriter, req *http.Request, repository, reference string) {
	key := repository + ":" + reference
	if strings.Contains(reference, ":") {
		key = repository + "@" + reference
	}
	if req.Method == http.MethodHead {
		r.count(HeadManifest)
	} else {
		r.count(GetManifest)
	}
	r.mu.Lock()
	m, ok := r.manifests[key]
	r.mu.Unlock()
	if !ok {
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", m.mediaType)
	w.Header().Set("Docker-Content-Digest", digest.Of(m.body).String())
	w.Header().Set("Content-Length", strconv.Itoa(len(m.body)))
	if req.Method != http.MethodHead {
		_, _ = w.Write(m.body)
	}
}

// serveBlob answers a blob GET, or redirects it to storage.
func (r *Registry) serveBlob(w http.ResponseWriter, req *http.Request, d digest.Digest) {
	r.count(GetBlob)
	if r.opts.RedirectBlobs {
		http.Redirect(w, req, r.server.URL+"/storage/"+d.String(), http.StatusTemporaryRedirect)
		return
	}
	r.sendBlob(w, d)
}

// sendBlob writes a blob's bytes, stopping halfway while bodies are held.
func (r *Registry) sendBlob(w http.ResponseWriter, d digest.Digest) {
	r.mu.Lock()
	data, ok := r.blobs[d]
	held := r.held
	r.mu.Unlock()
	if !ok {
		http.Error(w, "blob unknown", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	half := len(data) / 2
	_, _ = w.Write(data[:half])
	if held != nil {
		w.(http.Flusher).Flush()
		<-held
	}
	_, _ = w.Write(data[half:])
}
