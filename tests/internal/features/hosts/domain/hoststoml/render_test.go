// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hoststoml_test

// Verifies:
//   - A registry's hosts.toml names its real API as the server (Docker Hub's apart from its name),
//     limen as the pull host, and no dial_timeout (Docker ignores it)
//   - limenctl knows its own files, which mirror a file points at, and reads it back
//   - Files live where Docker's containerd image store reads them

import (
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/features/hosts/domain/hoststoml"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
)

func TestAHostsTomlSendsPullsToLimenFirst(t *testing.T) {
	got := string(hoststoml.Render("docker.io", "http://10.0.0.5:5000/"))
	for _, line := range []string{
		`server = "https://registry-1.docker.io"`,
		`[host."http://10.0.0.5:5000"]`,
		`capabilities = ["pull", "resolve"]`,
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %s in\n%s", line, got)
		}
	}
	ghcr := string(hoststoml.Render("ghcr.io", "http://10.0.0.5:5000"))
	if !strings.Contains(ghcr, `server = "https://ghcr.io"`) || strings.Contains(got+ghcr, "dial_timeout") {
		t.Errorf("ghcr.io:\n%s", ghcr)
	}
}

func TestLimenctlKnowsItsOwnFiles(t *testing.T) {
	ours := hoststoml.Render("quay.io", "http://10.0.0.5:5000")
	if !hoststoml.Ours(ours) || hoststoml.Ours([]byte("server = \"https://quay.io\"\n")) {
		t.Error("ownership misread")
	}
	if !hoststoml.PointsAt(ours, "http://10.0.0.5:5000/") || hoststoml.PointsAt(ours, "http://10.0.0.6:5000") {
		t.Error("mirror misread")
	}
	if layout.Linux().HostsToml("ghcr.io") != "/etc/docker/certs.d/ghcr.io/hosts.toml" {
		t.Error(layout.Linux().HostsToml("ghcr.io"))
	}
}

func TestTheMirrorAFileNamesIsRead(t *testing.T) {
	if got := hoststoml.Mirror(hoststoml.Render("ghcr.io", "http://192.0.2.10:5000/")); got != "http://192.0.2.10:5000" {
		t.Errorf("%q", got)
	}
	if got := hoststoml.Mirror([]byte("server = \"https://ghcr.io\"\n")); got != "" {
		t.Errorf("none: %q", got)
	}
}
