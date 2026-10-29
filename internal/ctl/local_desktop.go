// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

//go:build windows || darwin

// local_desktop.go wires limenctl to Docker Desktop: its files by their full paths in the user's
// home, restarted through docker desktop.
package ctl

import (
	"os"
	"path/filepath"

	"github.com/nauarch/limen/internal/adapters/host/desktop"
	"github.com/nauarch/limen/internal/adapters/host/files"
	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts"
)

// local is the host limenctl runs on: Docker Desktop, for the user running it.
func local() (files.Dir, hosts.Layout, host.Services, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return files.Dir{}, hosts.Layout{}, nil, err
	}
	return files.Dir{}, hosts.DesktopLayout(filepath.ToSlash(home)), desktop.Desktop{}, nil
}
