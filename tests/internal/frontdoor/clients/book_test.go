// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package clients_test

// Verifies:
//   - Each request is counted against its address, the time of the last kept
//   - An address asks what is known of itself, and only of itself; unknown ones have no requests
//   - A full book forgets the address heard from longest ago

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/frontdoor/clients"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
)

var start = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

// pull sends a pull request from addr through the book.
func pull(b *clients.Book, addr string) {
	r := httptest.NewRequest(http.MethodGet, "/v2/", nil)
	r.RemoteAddr = addr + ":40000"
	b.Note(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(httptest.NewRecorder(), r)
}

// me asks the book as addr.
func me(t *testing.T, b *clients.Book, addr string) ops.Client {
	r := httptest.NewRequest(http.MethodGet, "/api/clients/me", nil)
	r.RemoteAddr = addr + ":50000"
	w := httptest.NewRecorder()
	b.Me(w, r)
	var c ops.Client
	if err := json.NewDecoder(w.Body).Decode(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRequestsAreCountedByAddress(t *testing.T) {
	clock := kitclock.At(start)
	b := clients.New(0, clock)
	pull(b, "192.0.2.1")
	clock.Advance(time.Minute)
	pull(b, "192.0.2.1")
	pull(b, "192.0.2.2")
	if c := me(t, b, "192.0.2.1"); c.Address != "192.0.2.1" || c.Requests != 2 || !c.Last.Equal(start.Add(time.Minute)) {
		t.Errorf("%+v", c)
	}
	if c := me(t, b, "192.0.2.9"); c.Address != "192.0.2.9" || c.Requests != 0 || !c.Last.IsZero() {
		t.Errorf("unknown: %+v", c)
	}
}

func TestAFullBookForgetsTheStalest(t *testing.T) {
	clock := kitclock.At(start)
	b := clients.New(2, clock)
	for _, a := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		pull(b, a)
		clock.Advance(time.Second)
	}
	if me(t, b, "192.0.2.1").Requests != 0 || me(t, b, "192.0.2.2").Requests != 1 || me(t, b, "192.0.2.3").Requests != 1 {
		t.Error("the stalest address was not the one forgotten")
	}
}
