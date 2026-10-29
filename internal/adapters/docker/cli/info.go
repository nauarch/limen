// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// snapshotterType is the driver type Docker reports when it uses the containerd image store.
const snapshotterType = "io.containerd.snapshotter.v1"

// info is the part of `docker info` limenctl reads.
type info struct {
	ServerVersion   string
	Driver          string
	DriverStatus    [][2]string
	DockerRootDir   string
	OSType          string
	Architecture    string
	Containerd      struct{ Address string }
	SecurityOptions []string
	Swarm           struct{ LocalNodeState string }
	RegistryConfig  struct{ Mirrors []string }
}

// Info reads `docker info`.
func (c *Client) Info(ctx context.Context) (host.Info, error) {
	out, err := c.run(ctx, "info", "--format", "{{json .}}")
	if err != nil {
		return host.Info{}, err
	}
	var i info
	if err := json.Unmarshal(out, &i); err != nil {
		return host.Info{}, fmt.Errorf("docker info: %w: %w", errkind.ErrUpstream, err)
	}
	h := host.Info{
		Version:    i.ServerVersion,
		Driver:     i.Driver,
		RootDir:    i.DockerRootDir,
		Containerd: i.Containerd.Address,
		OS:         i.OSType,
		Arch:       i.Architecture,
		Swarm:      i.Swarm.LocalNodeState != "" && i.Swarm.LocalNodeState != "inactive",
		Mirrors:    i.RegistryConfig.Mirrors,
	}
	for _, kv := range i.DriverStatus {
		if kv[0] == "driver-type" && kv[1] == snapshotterType {
			h.Snapshotter = true
		}
	}
	for _, o := range i.SecurityOptions {
		if strings.HasPrefix(o, "name=userns") {
			h.UsernsRemap = true
		}
	}
	return h, nil
}
