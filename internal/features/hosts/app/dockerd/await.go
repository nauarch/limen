// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package dockerd

import (
	"context"
	"fmt"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
)

// poll is how often a restarting Docker is asked whether it is back.
const poll = time.Second

// Await waits up to within for Docker to answer again after a restart, then checks what it says:
// check's error is the answer when Docker is back, a timeout when it never comes.
func Await(ctx context.Context, engine host.Engine, within time.Duration, check func(host.Info) error) error {
	ctx, cancel := context.WithTimeout(ctx, within)
	defer cancel()
	for {
		info, err := engine.Info(ctx)
		if err == nil {
			return check(info)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("docker did not come back within %s: %w", within, err)
		case <-time.After(poll):
		}
	}
}
