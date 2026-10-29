// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/nauarch/limen/internal/kernel/build"
	"github.com/nauarch/limen/internal/server"
)

// version is the release, stamped at build time (-ldflags "-X main.version=v1.2.3").
var version = "dev"

// main reads the command line and runs limen until stopped by a signal. --version prints the
// version (anywhere among the arguments, as an image's entrypoint names --dir first);
// --healthcheck only asks a running limen whether it is healthy, for a container's health check;
// install and uninstall set it up as (or remove it from being) a systemd service.
func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if slices.Contains(os.Args[1:], "--version") || len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(build.About("limen", build.Version(version)))
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		os.Exit(healthcheck(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "install" {
		os.Exit(install(os.Args[2:], os.Stdout))
	}
	if len(os.Args) > 1 && os.Args[1] == "uninstall" {
		os.Exit(uninstall(os.Args[2:], os.Stdout))
	}
	o, err := parse(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	o.Version = build.Version(version)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, o, log); err != nil {
		log.Error("limen stopped", "err", err)
		stop()
		os.Exit(1)
	}
}
