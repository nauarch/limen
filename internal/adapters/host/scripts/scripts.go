// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package scripts

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// said is how much of a failing script's output its error carries, from the end.
const said = 2 << 10

// Exec runs the host's scripts as programs of their own.
type Exec struct{}

var _ host.Scripts = Exec{}

// Run runs the script at path in dir and waits for it; a failure carries the end of its output.
func (Exec) Run(ctx context.Context, path, dir string) error {
	cmd := exec.CommandContext(ctx, path)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		tail := out.Bytes()
		if len(tail) > said {
			tail = tail[len(tail)-said:]
		}
		return errkind.Command(path, string(tail), errkind.ErrUpstream, err)
	}
	return nil
}
