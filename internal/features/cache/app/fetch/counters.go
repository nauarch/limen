// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package fetch

import (
	"io"
	"sync/atomic"
)

// Counters count what the fetchers did since limen started; the cache reports them as its stats.
type Counters struct {
	BlobHits, BlobJoins, BlobMisses, BlobFailures atomic.Int64
	BytesServed, BytesFetched                     atomic.Int64
	TagsFresh, TagsChecked, TagsStale             atomic.Int64
}

// countedBlob is a stored blob whose reads count as bytes served; it still seeks, so range
// requests keep working.
type countedBlob struct {
	io.ReadSeekCloser
	served *atomic.Int64
}

// Read reads and counts.
func (c countedBlob) Read(p []byte) (int, error) {
	n, err := c.ReadSeekCloser.Read(p)
	c.served.Add(int64(n))
	return n, err
}
