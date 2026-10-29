// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package client_test

// Verifies:
//   - A prefetch is posted as JSON and its job read back; a job's progress is read by its ID
//   - limen refusing a prefetch is Upstream, with its own words

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/adapters/limen/client"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestAPrefetchIsStartedAndFollowed(t *testing.T) {
	var asked prefetch.Request
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/prefetch", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&asked); err != nil || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"j1","state":"running","images":2}`))
	})
	mux.HandleFunc("GET /api/prefetch/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + r.PathValue("id") + `","state":"done","fetched":9,"bytes":1024}`))
	})
	s := httptest.NewServer(mux)
	defer s.Close()
	c := client.New(s.URL)
	job, err := c.Prefetch(context.Background(), prefetch.Request{Images: []string{"nginx:alpine", "redis:8"}, Platforms: []string{"linux/arm64"}})
	if err != nil || job.ID != "j1" || job.State != prefetch.Running || !slices.Equal(asked.Images, []string{"nginx:alpine", "redis:8"}) || asked.Platforms[0] != "linux/arm64" {
		t.Fatalf("%+v %v (asked %+v)", job, err, asked)
	}
	if job, err = c.Job(context.Background(), "j1"); err != nil || job.State != prefetch.Done || job.Fetched != 9 {
		t.Errorf("%+v %v", job, err)
	}
}

func TestARefusedPrefetchIsUpstream(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "prefetch request names no image", http.StatusBadRequest)
	}))
	defer s.Close()
	_, err := client.New(s.URL).Prefetch(context.Background(), prefetch.Request{})
	if errkind.Of(err) != errkind.Upstream || !strings.Contains(err.Error(), "names no image") {
		t.Errorf("%v", err)
	}
}
