// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Runner runs the docker command line with args and gives what it printed.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Client is the host's Docker engine, read through the docker command line's JSON output, so
// limenctl talks to whatever engine the host's docker command does.
type Client struct {
	run Runner
}

var _ host.Engine = (*Client)(nil)

// New reads the engine with run; nil runs the docker command found on the PATH.
func New(run Runner) *Client {
	if run == nil {
		run = Exec
	}
	return &Client{run: run}
}

// Exec runs the docker command; a failure carries what it printed to stderr.
func Exec(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errkind.Command("docker "+strings.Join(args, " "), stderr.String(), errkind.ErrUnreachable, err)
	}
	return out, nil
}

// ids runs a listing command and gives the IDs it printed, once each.
func (c *Client) ids(ctx context.Context, args ...string) ([]string, error) {
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for id := range strings.FieldsSeq(string(out)) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// inspect runs an inspect command over args (its flags, then IDs) into v (a pointer to a slice).
func (c *Client) inspect(ctx context.Context, kind string, args []string, v any) error {
	out, err := c.run(ctx, append([]string{kind, "inspect"}, args...)...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("docker %s inspect: %w: %w", kind, errkind.ErrUpstream, err)
	}
	return nil
}
