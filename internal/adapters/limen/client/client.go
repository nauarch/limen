// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// timeout bounds each request to limen.
const timeout = 10 * time.Second

// Client reaches a limen over its ops API.
type Client struct {
	base string
	http *http.Client
}

var _ host.Limen = (*Client)(nil)

// New reaches the limen at base, such as http://10.0.0.5:5000.
func New(base string) *Client {
	return &Client{base: strings.TrimSuffix(base, "/"), http: &http.Client{Timeout: timeout}}
}

// Healthy asks limen's /healthz.
func (c *Client) Healthy(ctx context.Context) error {
	resp, err := c.get(ctx, "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return fmt.Errorf("limen is unhealthy (%s): %s: %w", resp.Status, strings.TrimSpace(string(body)), errkind.ErrUpstream)
	}
	return nil
}

// Registries asks limen which registries it serves.
func (c *Client) Registries(ctx context.Context) ([]string, error) {
	resp, err := c.get(ctx, "/api/registries")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("limen answered %s: %w", resp.Status, errkind.ErrUpstream)
	}
	var body struct {
		Registries []string `json:"registries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("limen's registries: %w: %w", errkind.ErrUpstream, err)
	}
	return body.Registries, nil
}

// get sends a GET to limen; no answer at all is ErrUnreachable.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, fmt.Errorf("limen %s: %w: %w", c.base, errkind.ErrInvalid, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("limen %s: %w: %w", c.base, errkind.ErrUnreachable, err)
	}
	return resp, nil
}
