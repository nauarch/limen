// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package client_test

// Verifies:
//   - Me reads what limen knows of this client from /api/clients/me
//   - A limen without that API is an Upstream error

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nauarch/limen/internal/adapters/limen/client"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestMeIsRead(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/clients/me", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"address":"192.0.2.7","requests":5,"last":"2026-10-29T12:00:00Z"}`))
	})
	s := httptest.NewServer(mux)
	defer s.Close()
	me, err := client.New(s.URL).Me(context.Background())
	if err != nil || me.Address != "192.0.2.7" || me.Requests != 5 || me.Last.IsZero() {
		t.Errorf("%+v %v", me, err)
	}
	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	if _, err := client.New(old.URL).Me(context.Background()); errkind.Of(err) != errkind.Upstream {
		t.Errorf("an old limen: %v", err)
	}
}
