// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package host

import (
	"context"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/contracts/prefetch"
)

// Files are the host's files, by absolute path. Missing ones are errkind.ErrNotFound.
type Files interface {
	Read(path string) ([]byte, error)
	// Write replaces a file whole (through a temporary file and a rename), making its folder.
	Write(path string, data []byte, perm uint32) error
	// List names a folder's entries.
	List(dir string) ([]string, error)
	// Free is the space left on the filesystem holding path, in bytes; -1 when unknown.
	Free(path string) (int64, error)
	// Size is the bytes a file or folder holds, all it contains counted.
	Size(path string) (int64, error)
	// Remove removes a file or a folder with all it contains; a missing one is no error.
	Remove(path string) error
	// MakeDir makes a folder and the folders above it; one that exists is no error.
	MakeDir(path string) error
}

// Services are the host's system services.
type Services interface {
	Restart(ctx context.Context, unit string) error
}

// Limen is the limen a host is pointed at.
type Limen interface {
	// Healthy fails when limen cannot be reached or cannot serve.
	Healthy(ctx context.Context) error
	// Registries are the registries it serves.
	Registries(ctx context.Context) ([]string, error)
	// Prefetch starts limen fetching images ahead of their pulls.
	Prefetch(ctx context.Context, r prefetch.Request) (prefetch.Job, error)
	// Job tells how a prefetch is going.
	Job(ctx context.Context, id string) (prefetch.Job, error)
	// Me is what limen knows of the address limenctl reaches it from: the pulls it has seen.
	Me(ctx context.Context) (ops.Client, error)
}

// Network is the host's network.
type Network interface {
	// Addresses are the IP addresses of the host's interfaces.
	Addresses() ([]string, error)
}

// Project is a compose project as Docker compose names it: its name, folder and compose files.
type Project struct {
	Name  string
	Dir   string
	Files []string
}

// Compose runs Docker compose on the host's projects.
type Compose interface {
	// Version is the compose plugin's version; it fails where docker compose cannot run.
	Version(ctx context.Context) (string, error)
	// Stop stops a project's containers, keeping them.
	Stop(ctx context.Context, p Project) error
	// Up creates and starts a project's containers, pulling or building their images as needed.
	Up(ctx context.Context, p Project) error
}

// Scripts run the host's own scripts.
type Scripts interface {
	// Run runs the script at path in dir and waits for it; failing is an error with what it said.
	Run(ctx context.Context, path, dir string) error
}
