// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package host is the contract of what limenctl reads and changes on a Docker host: the Docker
// engine, the host's files and services, and the limen it points the host at.
package host

import "context"

// Engine is the host's Docker engine.
type Engine interface {
	Info(ctx context.Context) (Info, error)
	Containers(ctx context.Context) ([]Container, error)
	Images(ctx context.Context) ([]Image, error)
	// PruneImages removes every image no container uses.
	PruneImages(ctx context.Context) error
	// SaveImages writes the named images to a file at path, layers they share once.
	SaveImages(ctx context.Context, refs []string, path string) error
	// LoadImages reads images back from a file SaveImages wrote.
	LoadImages(ctx context.Context, path string) error
	// Pull pulls an image as docker pull does, through whatever mirrors the engine is set to.
	Pull(ctx context.Context, ref string) error
	// RemoveImage removes an image by its reference.
	RemoveImage(ctx context.Context, ref string) error
}

// Info is what the engine says about itself: its version, its storage driver and whether that is
// containerd's snapshotter (the containerd image store), its data root, the containerd it talks to
// (by socket), its OS and architecture as Docker names them (linux, x86_64), swarm and
// user-namespace state, and the registry mirrors it is set to.
type Info struct {
	Version     string
	Driver      string
	Snapshotter bool
	RootDir     string
	Containerd  string
	OS          string
	Arch        string
	Swarm       bool
	UsernsRemap bool
	Mirrors     []string
}

// Container is a container as limenctl needs it: who it is, what it runs, whether it runs, its
// labels (compose's among them), whether it uses the host's network, the host ports it publishes,
// and the bytes it wrote outside its volumes (its writable layer, -1 when unknown).
type Container struct {
	ID          string
	Name        string
	Image       string
	ImageID     string
	Running     bool
	Labels      map[string]string
	HostNetwork bool
	Ports       []int
	Written     int64
}

// Image is a local image: its ID, names, registry digests and size.
type Image struct {
	ID      string
	Tags    []string
	Digests []string
	Size    int64
}
