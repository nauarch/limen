// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package clients notes which addresses pull through limen, so a host can ask whether its own
// pulls arrive: GET /api/clients/me.
package clients

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/kernel/clock"
)

// DefaultMax is how many addresses the book keeps when not told otherwise; past it, the one
// heard from longest ago is forgotten.
const DefaultMax = 4096

// Book counts the pull requests from each client address, in memory.
type Book struct {
	mu    sync.Mutex
	seen  map[string]ops.Client
	max   int
	clock clock.Clock
}

// New keeps up to max addresses (0 for DefaultMax), timed by clock (nil for the system's).
func New(max int, c clock.Clock) *Book {
	if max <= 0 {
		max = DefaultMax
	}
	if c == nil {
		c = clock.System{}
	}
	return &Book{seen: map[string]ops.Client{}, max: max, clock: c}
}

// Note counts each request next serves against the address it came from.
func (b *Book) Note(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.add(address(r))
		next.ServeHTTP(w, r)
	})
}

// Me answers what the book knows of the asking address, with no requests when nothing.
func (b *Book) Me(w http.ResponseWriter, r *http.Request) {
	a := address(r)
	b.mu.Lock()
	c, ok := b.seen[a]
	b.mu.Unlock()
	if !ok {
		c = ops.Client{Address: a}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c)
}

// add counts one request from a, forgetting the stalest address when the book is full.
func (b *Book) add(a string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.seen[a]
	if !ok && len(b.seen) >= b.max {
		var stalest string
		for k, v := range b.seen {
			if stalest == "" || v.Last.Before(b.seen[stalest].Last) {
				stalest = k
			}
		}
		delete(b.seen, stalest)
	}
	c.Address, c.Requests, c.Last = a, c.Requests+1, b.clock.Now()
	b.seen[a] = c
}

// address is the request's client address, without its port.
func address(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}
