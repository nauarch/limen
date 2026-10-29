// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"slices"
	"strconv"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
)

// The containerd a Docker engine may talk to, and where each keeps its data.
const (
	// ContainerdConfig is the system containerd's settings, which may move its data root.
	ContainerdConfig = "/etc/containerd/config.toml"
	// systemRoot is the system containerd's data root unless its settings move it.
	systemRoot = "/var/lib/containerd"
	// managedRoot is the data root of a containerd dockerd runs itself, under Docker's own root.
	managedRoot = "/containerd"
)

// systemSockets are the system containerd's sockets; any other is one dockerd runs itself.
var systemSockets = []string{"/run/containerd/containerd.sock", "/var/run/containerd/containerd.sock"}

// StoreRoot is where the containerd image store keeps its data on this engine: the system
// containerd's root (from its settings, else /var/lib/containerd), or, for a containerd dockerd
// runs itself, the containerd folder under Docker's root.
func StoreRoot(engine host.Info, containerdConfig []byte) string {
	if engine.Containerd != "" && !slices.Contains(systemSockets, engine.Containerd) && engine.RootDir != "" {
		return strings.TrimSuffix(engine.RootDir, "/") + managedRoot
	}
	if root := configRoot(containerdConfig); root != "" {
		return root
	}
	return systemRoot
}

// configRoot reads the top-level root = "…" of a containerd config.toml, "" when it has none.
func configRoot(config []byte) string {
	for line := range strings.Lines(string(config)) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			return ""
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "root" {
			continue
		}
		if root, err := strconv.Unquote(strings.TrimSpace(value)); err == nil {
			return root
		}
	}
	return ""
}
