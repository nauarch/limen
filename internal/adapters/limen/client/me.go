// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Me asks limen what it knows of the address this client reaches it from.
func (c *Client) Me(ctx context.Context) (ops.Client, error) {
	resp, err := c.get(ctx, "/api/clients/me")
	if err != nil {
		return ops.Client{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ops.Client{}, fmt.Errorf("limen answered %s (a limen too old to count its clients?): %w", resp.Status, errkind.ErrUpstream)
	}
	var me ops.Client
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return ops.Client{}, fmt.Errorf("limen's answer about this client: %w: %w", errkind.ErrUpstream, err)
	}
	return me, nil
}
