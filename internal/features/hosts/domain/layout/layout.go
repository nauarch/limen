// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package layout names where a Docker host keeps what limenctl reads and writes: a Linux host's
// /etc/docker, or Docker Desktop's .docker folder in the user's home.
package layout

import "path"

// Layout is where the host keeps Docker's registry files and limenctl its own, as absolute paths
// with forward slashes.
type Layout struct {
	// CertsDir holds a folder per registry, with its hosts.toml.
	CertsDir string
	// DaemonJSON is Docker's daemon.json.
	DaemonJSON string
	// StateDir is limenctl's own: backups, and on Linux the migration's journal.
	StateDir string
	// Desktop is Docker Desktop: it copies CertsDir into its VM only when it starts, so a changed
	// hosts.toml takes a restart, and it switches image stores in its own settings.
	Desktop bool
}

// Linux is a Linux host's layout, its files read from /.
func Linux() Layout {
	return Layout{CertsDir: "/etc/docker/certs.d", DaemonJSON: "/etc/docker/daemon.json", StateDir: "/var/lib/limenctl"}
}

// Desktop is Docker Desktop's layout in the user's home, given with forward slashes (C:/Users/me).
func Desktop(home string) Layout {
	docker := path.Join(home, ".docker")
	return Layout{CertsDir: path.Join(docker, "certs.d"), DaemonJSON: path.Join(docker, "daemon.json"), StateDir: path.Join(docker, "limenctl"), Desktop: true}
}

// HostsToml is the hosts.toml for a registry.
func (l Layout) HostsToml(registry string) string {
	return path.Join(l.CertsDir, registry, "hosts.toml")
}

// Backups is where replaced files are kept, named by their path and the time.
func (l Layout) Backups() string { return path.Join(l.StateDir, "backups") }
