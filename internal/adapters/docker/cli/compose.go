// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
)

// Compose runs Docker compose through the docker command line's compose plugin.
type Compose struct {
	run Runner
}

var _ host.Compose = (*Compose)(nil)

// NewCompose runs compose with run; nil runs the docker command found on the PATH.
func NewCompose(run Runner) *Compose {
	if run == nil {
		run = Exec
	}
	return &Compose{run: run}
}

// Version asks the compose plugin for its version.
func (c *Compose) Version(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "compose", "version", "--short")
	return strings.TrimSpace(string(out)), err
}

// Stop stops a project's containers, keeping them.
func (c *Compose) Stop(ctx context.Context, p host.Project) error {
	_, err := c.run(ctx, args(p, "stop")...)
	return err
}

// Up creates and starts a project's containers in the background, pulling or building their
// images as needed.
func (c *Compose) Up(ctx context.Context, p host.Project) error {
	_, err := c.run(ctx, args(p, "up", "--detach", "--quiet-pull")...)
	return err
}

// Pull pulls a project's images, passing the services it builds.
func (c *Compose) Pull(ctx context.Context, p host.Project) error {
	_, err := c.run(ctx, args(p, "pull", "--ignore-buildable", "--quiet")...)
	return err
}

// args is a compose command line naming the project, its folder and its files, as compose
// recorded them on its containers.
func args(p host.Project, command ...string) []string {
	out := []string{"compose", "--project-name", p.Name, "--project-directory", p.Dir}
	for _, f := range p.Files {
		out = append(out, "--file", f)
	}
	return append(out, command...)
}
