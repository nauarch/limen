// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package systemd

import (
	"bytes"
	"cmp"
	"context"
	"os/exec"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// DefaultTimeout bounds a restart: systemd's own limits end most well before it, but a unit that
// never settles must not hold limenctl forever.
const DefaultTimeout = 5 * time.Minute

// Systemd restarts the host's services through systemctl; Command is the systemctl to run (""
// for the one on the PATH) and Timeout bounds each restart (zero for DefaultTimeout).
type Systemd struct {
	Command string
	Timeout time.Duration
}

var _ host.Services = Systemd{}

// Restart restarts a unit and waits for systemctl to say it is done.
func (s Systemd) Restart(ctx context.Context, unit string) error {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(s.Timeout, DefaultTimeout))
	defer cancel()
	cmd := exec.CommandContext(ctx, cmp.Or(s.Command, "systemctl"), "restart", unit)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return errkind.Command("systemctl restart "+unit, stderr.String(), errkind.ErrUpstream, cmp.Or(ctx.Err(), err))
	}
	return nil
}
