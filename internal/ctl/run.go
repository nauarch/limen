// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ctl

import (
	"context"
	"fmt"
	"io"

	dockercli "github.com/nauarch/limen/internal/adapters/docker/cli"
	"github.com/nauarch/limen/internal/adapters/host/network"
	"github.com/nauarch/limen/internal/adapters/host/scripts"
	limenclient "github.com/nauarch/limen/internal/adapters/limen/client"
	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts"
	"github.com/nauarch/limen/internal/frontdoor/cli"
)

// Run composes limenctl (the local host's layout, files and services, its docker command and
// compose, network and scripts, the limen client, the hosts feature and the command line) and
// runs one command, giving its exit code; version is the build's.
func Run(ctx context.Context, version string, args []string, out, errOut io.Writer) int {
	fs, lay, services, err := local()
	if err != nil {
		fmt.Fprintln(errOut, "limenctl:", err)
		return 1
	}
	h, err := hosts.Compose(hosts.Options{
		Layout:   lay,
		Engine:   dockercli.New(nil),
		Files:    fs,
		Network:  network.Interfaces{},
		Services: services,
		Compose:  dockercli.NewCompose(nil),
		Scripts:  scripts.Exec{},
		Limen:    func(url string) host.Limen { return limenclient.New(url) },
	})
	if err != nil {
		fmt.Fprintln(errOut, "limenctl:", err)
		return 1
	}
	return cli.Run(ctx, args, cli.Options{Planner: h, Migrator: h, Version: version, Out: out, Err: errOut})
}
