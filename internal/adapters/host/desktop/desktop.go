// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package desktop restarts Docker Desktop through its own command line, docker desktop.
package desktop

import (
	"bytes"
	"cmp"
	"context"
	"os/exec"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// DefaultTimeout bounds a restart: Docker Desktop restarts its whole VM, which takes a while,
// but one that never comes back must not hold limenctl forever.
const DefaultTimeout = 5 * time.Minute

// Desktop restarts Docker Desktop; Command is the docker to run ("" for the one on the PATH) and
// Timeout bounds each restart (zero for DefaultTimeout).
type Desktop struct {
	Command string
	Timeout time.Duration
}

var _ host.Services = Desktop{}

// Restart restarts Docker Desktop, the one service it runs, whatever the unit is named, and
// waits for docker desktop to say it is done.
func (d Desktop) Restart(ctx context.Context, _ string) error {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(d.Timeout, DefaultTimeout))
	defer cancel()
	cmd := exec.CommandContext(ctx, cmp.Or(d.Command, "docker"), "desktop", "restart")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return errkind.Command("docker desktop restart", out.String(), errkind.ErrUpstream, cmp.Or(ctx.Err(), err))
	}
	return nil
}
