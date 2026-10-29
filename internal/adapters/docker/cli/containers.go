// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"strconv"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
)

// container is the part of `docker container inspect` limenctl reads.
type container struct {
	ID     string `json:"Id"`
	Name   string
	Image  string
	Config struct {
		Image  string
		Labels map[string]string
	}
	SizeRw     *int64
	State      struct{ Running bool }
	HostConfig struct {
		NetworkMode  string
		PortBindings map[string][]struct{ HostPort string }
	}
}

// Containers reads every container, running or not, with the size of its writable layer.
func (c *Client) Containers(ctx context.Context) ([]host.Container, error) {
	ids, err := c.ids(ctx, "ps", "--all", "--quiet", "--no-trunc")
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	var raw []container
	if err := c.inspect(ctx, "container", append([]string{"--size"}, ids...), &raw); err != nil {
		return nil, err
	}
	out := make([]host.Container, 0, len(raw))
	for _, r := range raw {
		h := host.Container{
			ID:          r.ID,
			Name:        strings.TrimPrefix(r.Name, "/"),
			Image:       r.Config.Image,
			ImageID:     r.Image,
			Running:     r.State.Running,
			Labels:      r.Config.Labels,
			HostNetwork: r.HostConfig.NetworkMode == "host",
			Written:     -1,
		}
		if r.SizeRw != nil {
			h.Written = *r.SizeRw
		}
		for _, bindings := range r.HostConfig.PortBindings {
			for _, b := range bindings {
				if p, err := strconv.Atoi(b.HostPort); err == nil {
					h.Ports = append(h.Ports, p)
				}
			}
		}
		out = append(out, h)
	}
	return out, nil
}
