// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

//go:build !(windows || darwin)

// local_linux.go wires limenctl to a Linux host: its files from /, Docker restarted through
// systemd.
package ctl

import (
	"github.com/nauarch/limen/internal/adapters/host/files"
	"github.com/nauarch/limen/internal/adapters/host/systemd"
	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts"
)

// local is the host limenctl runs on: a Linux host, as root.
func local() (files.Dir, hosts.Layout, host.Services, error) {
	return files.Dir{Root: "/"}, hosts.LinuxLayout(), systemd.Systemd{}, nil
}
