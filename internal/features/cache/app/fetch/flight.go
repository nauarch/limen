// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nauarch/limen/internal/contracts/storage"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// chunk is how much of a blob is copied from upstream at a time.
const chunk = 256 << 10

// flight is one blob download that every concurrent pull of that blob shares. started closes once
// upstream has answered: with startErr, or with the size and the pending blob being written. From
// then on written counts the bytes on disk, and done says the download has landed (committed, or
// failed with err).
type flight struct {
	started  chan struct{}
	startErr error
	size     int64

	mu      sync.Mutex
	more    *sync.Cond
	pending storage.Pending
	written int64
	done    bool
	err     error
}

// newFlight is a download not yet answered.
func newFlight() *flight {
	f := &flight{started: make(chan struct{})}
	f.more = sync.NewCond(&f.mu)
	return f
}

// fail ends a download upstream refused or the store could not begin.
func (f *flight) fail(err error) {
	f.startErr = err
	f.mu.Lock()
	f.done, f.err = true, err
	f.mu.Unlock()
	close(f.started)
}

// begin lets the pulls waiting on the download read it.
func (f *flight) begin(size int64, p storage.Pending) {
	f.size, f.pending = size, p
	close(f.started)
}

// fill copies body into the pending blob, waking readers after each chunk, and checks that what
// arrived has the digest and size it should. A body that delivers nothing for stallAfter is closed
// and the download fails, so a stuck upstream never holds pulls forever. Bytes read count into
// fetched.
func (f *flight) fill(body io.ReadCloser, want digest.Digest, stallAfter time.Duration, fetched *atomic.Int64) error {
	var stalled atomic.Bool
	watchdog := time.AfterFunc(stallAfter, func() {
		stalled.Store(true)
		_ = body.Close()
	})
	defer watchdog.Stop()
	v := digest.NewVerifier(want)
	buf := make([]byte, chunk)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			watchdog.Reset(stallAfter)
			fetched.Add(int64(n))
			if _, err := f.pending.Write(buf[:n]); err != nil {
				return fmt.Errorf("writing blob %s: %w", want, err)
			}
			_, _ = v.Write(buf[:n])
			f.mu.Lock()
			f.written += int64(n)
			f.more.Broadcast()
			f.mu.Unlock()
		}
		if rerr == io.EOF && !stalled.Load() {
			break
		}
		if stalled.Load() {
			return fmt.Errorf("blob %s: upstream sent nothing for %s: %w", want, stallAfter, errkind.ErrUnreachable)
		}
		if rerr != nil {
			return fmt.Errorf("reading blob %s from upstream: %w: %w", want, errkind.ErrUnreachable, rerr)
		}
	}
	if f.size >= 0 && v.Size() != f.size {
		return fmt.Errorf("blob %s: %d bytes arrived, upstream announced %d: %w", want, v.Size(), f.size, errkind.ErrUpstream)
	}
	return v.Check()
}

// finish lands the download: commits the blob when err is nil, throws it away otherwise, and
// wakes every reader.
func (f *flight) finish(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		err = f.pending.Commit()
	} else {
		_ = f.pending.Abort()
	}
	f.done, f.err = true, err
	f.more.Broadcast()
}
