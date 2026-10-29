// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/server/service"
)

// defaultDir is the store limen install uses when --dir is not given.
const defaultDir = "/var/lib/limen"

// install installs limen as a systemd service serving with the flags given (as limen serves with
// them); uninstall removes it. Both print their steps, and take them only with --yes.
func install(args []string, out io.Writer) int {
	yes := slices.Contains(args, "--yes")
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--yes" || a == "--init" })
	if !slices.ContainsFunc(args, func(a string) bool { return a == "--dir" || strings.HasPrefix(a, "--dir=") }) {
		args = append([]string{"--dir=" + defaultDir}, args...)
	}
	o, err := parse(args)
	if err != nil {
		return 2
	}
	in, err := installer(o.Addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "limen install:", err)
		return 1
	}
	steps, err := in.Install(context.Background(), o.Dir, args)
	return take(steps, err, yes, "install", out)
}

// uninstall removes the service limen install made.
func uninstall(args []string, out io.Writer) int {
	in, err := installer("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "limen uninstall:", err)
		return 1
	}
	steps, err := in.Uninstall(context.Background())
	if err == nil && len(steps) == 0 {
		fmt.Fprintln(out, "limen is not installed as a service here: nothing to do.")
		return 0
	}
	return take(steps, err, slices.Contains(args, "--yes"), "uninstall", out)
}

// take prints the steps and, with yes, takes them.
func take(steps []service.Step, err error, yes bool, what string, out io.Writer) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "limen %s: %v\n", what, err)
		return 1
	}
	for i, s := range steps {
		fmt.Fprintf(out, "%d. %s\n", i+1, s.Say)
	}
	if !yes {
		fmt.Fprintf(out, "\nDry run: nothing was changed. Run again with --yes to %s.\n", what)
		return 0
	}
	fmt.Fprintln(out)
	for _, s := range steps {
		first, _, _ := strings.Cut(s.Say, "\n")
		fmt.Fprintf(out, "-> %s\n", first)
		if err := s.Do(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "limen %s: %s: %v\n", what, first, err)
			return 1
		}
	}
	fmt.Fprintln(out, "\nDone.")
	return 0
}

// installer is the installer for this host, checking limen's health at addr (":5000" when "").
func installer(addr string) (service.Installer, error) {
	self, err := os.Executable()
	if err != nil {
		return service.Installer{}, err
	}
	port := "5000"
	if _, p, ok := strings.Cut(addr, ":"); ok && p != "" {
		port = p
	}
	url := "http://127.0.0.1:" + port + "/healthz"
	return service.Installer{Root: "/", Self: self, Run: run, Healthy: func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s answered %s", url, resp.Status)
		}
		return nil
	}}, nil
}

// run runs a command; a failure carries what it printed.
func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return nil, errkind.Command(strings.Join(append([]string{name}, args...), " "), string(out), errkind.ErrUpstream, err)
	}
	return out, nil
}
