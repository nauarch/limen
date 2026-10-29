// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package service_test

// Verifies:
//   - The unit runs limen as its user with its arguments, restarts it always and without limit,
//     and lets it write only to its store
//   - Arguments with spaces are quoted as systemd reads them; a store under /home keeps it visible
//   - limen install knows its own unit file

import (
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/server/service"
)

func TestTheUnitRunsLimenSafely(t *testing.T) {
	got := string(service.Render(service.Unit{Binary: "/usr/local/bin/limen", Args: []string{"--dir=/var/lib/limen", "--max-size=300G"}, User: "limen", Dir: "/var/lib/limen"}))
	for _, line := range []string{
		"ExecStart=/usr/local/bin/limen --dir=/var/lib/limen --max-size=300G", "User=limen", "Restart=always",
		"StartLimitIntervalSec=0", "Type=notify", "WatchdogSec=90", "ProtectSystem=strict", "ProtectHome=yes", "ReadWritePaths=/var/lib/limen", "WantedBy=multi-user.target",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("missing %s in\n%s", line, got)
		}
	}
	if !service.Ours([]byte(got)) || service.Ours([]byte("[Unit]\n")) {
		t.Error("ownership misread")
	}
}

func TestArgumentsAreQuotedAndHomeStaysVisible(t *testing.T) {
	got := string(service.Render(service.Unit{Binary: "/usr/local/bin/limen", Args: []string{"--dir=/home/me/limen store"}, User: "limen", Dir: "/home/me/limen store"}))
	if !strings.Contains(got, `ExecStart=/usr/local/bin/limen "--dir=/home/me/limen store"`) || strings.Contains(got, "ProtectHome") {
		t.Errorf("\n%s", got)
	}
}
