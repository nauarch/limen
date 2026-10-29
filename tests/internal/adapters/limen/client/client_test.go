// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package client_test

// Verifies:
//   - A healthy limen answers /healthz; an unhealthy one is Upstream, with what it said
//   - The registries limen serves are read from /api/registries
//   - A limen that is not there is Unreachable; a garbled or refused answer is Upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/adapters/limen/client"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// limen serves /healthz with the health status and /api/registries with body.
func limen(t *testing.T, health int, body string) string {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(health)
		_, _ = w.Write([]byte("store: read-only\n"))
	})
	mux.HandleFunc("GET /api/registries", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) })
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s.URL
}

func TestAHealthyLimenSaysWhatItServes(t *testing.T) {
	c := client.New(limen(t, http.StatusOK, `{"registries":["docker.io","ghcr.io"]}`) + "/")
	if err := c.Healthy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Registries(context.Background()); err != nil || !slices.Equal(got, []string{"docker.io", "ghcr.io"}) {
		t.Errorf("%v %v", got, err)
	}
}

func TestAnUnhealthyLimenIsUpstream(t *testing.T) {
	err := client.New(limen(t, http.StatusServiceUnavailable, "")).Healthy(context.Background())
	if errkind.Of(err) != errkind.Upstream || !strings.Contains(err.Error(), "store: read-only") {
		t.Errorf("%v", err)
	}
}

func TestAMissingGarbledOrRefusingLimenIsAnError(t *testing.T) {
	if _, err := client.New(limen(t, http.StatusOK, "{")).Registries(context.Background()); errkind.Of(err) != errkind.Upstream {
		t.Errorf("garbled: %v", err)
	}
	if _, err := client.New(limen(t, http.StatusOK, "") + "/elsewhere").Registries(context.Background()); errkind.Of(err) != errkind.Upstream {
		t.Errorf("refused: %v", err)
	}
	s := httptest.NewServer(http.NotFoundHandler())
	s.Close()
	if err := client.New(s.URL).Healthy(context.Background()); errkind.Of(err) != errkind.Unreachable {
		t.Errorf("gone: %v", err)
	}
}
