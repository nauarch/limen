// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package hostplan is the contract between limenctl's command line and the hosts feature: the
// report on a Docker host, the changes configuring it would make, and the planner that makes both.
package hostplan

import "context"

// Planner reads a host and changes it.
type Planner interface {
	// Report reads the host; with a limen URL it also checks the host against that limen.
	Report(ctx context.Context, limen string) (Report, error)
	// Configure works out the changes pointing the host at a limen, without making them.
	Configure(ctx context.Context, r ConfigureRequest) (Changes, error)
	// Unconfigure works out the changes taking limenctl's configuration off the host, without
	// making them; limen, when given, also leaves daemon.json's mirrors.
	Unconfigure(ctx context.Context, limen string) (Changes, error)
	// Apply makes the changes, backing up what it replaces, and restarts Docker when they ask.
	Apply(ctx context.Context, c Changes) error
	// Verify pulls a small image and asks the limen whether the pull reached it.
	Verify(ctx context.Context, limen string) (Verification, error)
}

// Verification is how a pull went: the image pulled, the requests from this host limen counted
// during it (none means the pull went around limen), and whether the image was removed again, as
// the host did not have it before.
type Verification struct {
	Image    string
	Requests int64
	Removed  bool
}

// Levels of a finding: Info is worth knowing, Warn needs a look, Block stops a migration.
const (
	Info  = "info"
	Warn  = "warn"
	Block = "block"
)

// Finding is one thing the report says about the host.
type Finding struct {
	Level string
	Text  string
}

// Report is a host as limenctl sees it.
type Report struct {
	Version  string
	Store    string // "containerd" or "classic"
	Projects []Project
	// Loose are the containers not made by compose.
	Loose  []string
	InUse  Images
	Unused Images
	Disk   []Disk
	Mirror Mirror
	// Findings are ordered most serious first.
	Findings []Finding
}

// Project is a compose project on the host: its folder, compose files, how many of its
// containers run, and the shell scripts beside it (which may wrap compose with their own steps).
type Project struct {
	Name    string
	Dir     string
	Files   []string
	Running int
	Total   int
	Scripts []string
}

// Images counts images and their bytes.
type Images struct {
	Count int
	Bytes int64
}

// Disk is the space left where Docker keeps its data (-1 when unknown).
type Disk struct {
	Path string
	Free int64
}

// Mirror is how the host stands towards a limen: the limen asked about ("" when none), why it
// could not be reached ("" when it was), the registries it serves, those whose hosts.toml points
// at it, and the registry mirrors Docker's daemon is set to.
type Mirror struct {
	Limen       string
	Unreachable string
	Registries  []string
	Configured  []string
	Daemon      []string
}

// ConfigureRequest asks to point the host at a limen; Force replaces hosts.toml files limenctl
// did not write.
type ConfigureRequest struct {
	Limen string
	Force bool
}

// Changes are what configuring would do: files to write, files left alone and why, and whether
// Docker must restart (daemon.json changed).
type Changes struct {
	Writes  []Write
	Skipped []string
	Restart bool
	Limen   string
}

// Write is one file to write: what it holds now and will hold, nil for none (a file missing
// before, or removed after).
type Write struct {
	Path   string
	Before []byte
	After  []byte
}
