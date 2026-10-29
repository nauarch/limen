// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package layout_test

// Verifies:
//   - A Linux host keeps Docker's files in /etc/docker and limenctl's in /var/lib/limenctl
//   - Docker Desktop keeps them in .docker under the home, by full path, and says it is Desktop

import (
	"testing"

	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
)

func TestLinuxKeepsItsFilesInEtcDocker(t *testing.T) {
	l := layout.Linux()
	if l.HostsToml("ghcr.io") != "/etc/docker/certs.d/ghcr.io/hosts.toml" || l.DaemonJSON != "/etc/docker/daemon.json" || l.Backups() != "/var/lib/limenctl/backups" || l.Desktop {
		t.Errorf("%+v", l)
	}
}

func TestDesktopKeepsItsFilesInTheHome(t *testing.T) {
	l := layout.Desktop("C:/Users/me")
	if l.HostsToml("ghcr.io") != "C:/Users/me/.docker/certs.d/ghcr.io/hosts.toml" || l.DaemonJSON != "C:/Users/me/.docker/daemon.json" || l.Backups() != "C:/Users/me/.docker/limenctl/backups" || !l.Desktop {
		t.Errorf("%+v", l)
	}
}
