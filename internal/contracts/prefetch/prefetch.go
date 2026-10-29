// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package prefetch is the contract of warming the cache ahead of pulls: what limenctl asks limen
// to fetch, and how the job reports back. Both sides speak it as JSON.
package prefetch

import "context"

// Request asks for images to be fetched: references such as nginx:1.27 or
// ghcr.io/me/app@sha256:…, and the platforms (os/arch[/variant], such as linux/arm64) whose
// variants of multi-platform images are wanted. No platforms means every one.
type Request struct {
	Images    []string `json:"images"`
	Platforms []string `json:"platforms,omitempty"`
}

// The states of a job.
const (
	Running = "running"
	Done    = "done"
	Failed  = "failed"
)

// Job is a prefetch's progress: its blobs found, fetched and their bytes, and every image or blob
// that could not be fetched. Its state is Done when all were fetched, Failed when any could not be.
type Job struct {
	ID        string   `json:"id"`
	State     string   `json:"state"`
	Images    int      `json:"images"`
	Blobs     int      `json:"blobs"`
	Fetched   int      `json:"fetched"`
	Bytes     int64    `json:"bytes"`
	Failures  []string `json:"failures,omitempty"`
	StartedAt string   `json:"startedAt"`
}

// Prefetcher starts prefetch jobs and tells how they are going. Unknown jobs are
// errkind.ErrNotFound.
type Prefetcher interface {
	Start(ctx context.Context, r Request) (Job, error)
	Job(id string) (Job, error)
}
