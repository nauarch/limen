// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Prefetch starts limen fetching images ahead of their pulls.
func (c *Client) Prefetch(ctx context.Context, r prefetch.Request) (prefetch.Job, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return prefetch.Job{}, fmt.Errorf("prefetch request: %w: %w", errkind.ErrInvalid, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/prefetch", bytes.NewReader(body))
	if err != nil {
		return prefetch.Job{}, fmt.Errorf("limen %s: %w: %w", c.base, errkind.ErrInvalid, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return prefetch.Job{}, fmt.Errorf("limen %s: %w: %w", c.base, errkind.ErrUnreachable, err)
	}
	return job(resp, http.StatusAccepted)
}

// Job tells how a prefetch is going.
func (c *Client) Job(ctx context.Context, id string) (prefetch.Job, error) {
	resp, err := c.get(ctx, "/api/prefetch/"+url.PathEscape(id))
	if err != nil {
		return prefetch.Job{}, err
	}
	return job(resp, http.StatusOK)
}

// job reads a prefetch job from an answer that should have status want; limen's own words come
// with any other.
func job(resp *http.Response, want int) (prefetch.Job, error) {
	defer resp.Body.Close()
	if resp.StatusCode != want {
		said, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return prefetch.Job{}, fmt.Errorf("limen answered %s: %s: %w", resp.Status, strings.TrimSpace(string(said)), errkind.ErrUpstream)
	}
	var j prefetch.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return prefetch.Job{}, fmt.Errorf("limen's prefetch job: %w: %w", errkind.ErrUpstream, err)
	}
	return j, nil
}
