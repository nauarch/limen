// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package prefetch

import (
	"slices"
	"sync"

	contract "github.com/nauarch/limen/internal/contracts/prefetch"
)

// job is one prefetch's progress, shared between its run and whoever asks how it goes.
type job struct {
	mu sync.Mutex
	pj contract.Job
}

// snapshot is the job as it is now.
func (j *job) snapshot() contract.Job {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := j.pj
	out.Failures = slices.Clone(j.pj.Failures)
	return out
}

// found adds blobs to fetch.
func (j *job) found(n int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.pj.Blobs += n
}

// fetched counts a blob fetched and its bytes.
func (j *job) fetched(bytes int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.pj.Fetched++
	j.pj.Bytes += bytes
}

// failed notes what could not be fetched.
func (j *job) failed(what string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.pj.Failures = append(j.pj.Failures, what)
}

// end sets the final state: done, or failed when anything was not fetched.
func (j *job) end() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.pj.State = contract.Done
	if len(j.pj.Failures) > 0 {
		j.pj.State = contract.Failed
	}
}
