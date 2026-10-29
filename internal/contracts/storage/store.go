// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package storage is the contract of where the cache keeps what it pulled: blobs, manifests and
// what tags pointed at. A disk adapter implements it.
package storage

import (
	"io"
	"time"

	"github.com/nauarch/limen/internal/kernel/digest"
)

// Store keeps blobs, manifests and what tags pointed at. Missing things are errkind.ErrNotFound.
type Store interface {
	// Blob opens a committed blob and tells its size.
	Blob(d digest.Digest) (io.ReadSeekCloser, int64, error)
	// Begin starts writing a blob; nothing is visible until the Pending is committed.
	Begin(d digest.Digest) (Pending, error)
	// Touch records that a blob was used at now, for eviction to keep the most used.
	Touch(d digest.Digest, now time.Time) error
	// Blobs lists every committed blob with its size and last use.
	Blobs() ([]Entry, error)
	// RemoveBlob deletes a committed blob.
	RemoveBlob(d digest.Digest) error
	// Manifest reads a manifest by digest: its media type and bytes.
	Manifest(d digest.Digest) (mediaType string, body []byte, err error)
	// PutManifest keeps a manifest under its digest.
	PutManifest(d digest.Digest, mediaType string, body []byte) error
	// Tag reads what a registry's repository tag last pointed at.
	Tag(registry, repository, tag string) (TagRecord, error)
	// PutTag records what a tag points at now.
	PutTag(registry, repository, tag string, r TagRecord) error
	// Check fails when the store cannot be written to, so health checks see a lost share.
	Check() error
}

// Pending is a blob being written. Its bytes can be read back while it is written, so pulls that
// join a running download stream what has arrived. Commit makes it visible under its digest;
// Abort throws it away. Either closes it.
type Pending interface {
	io.Writer
	io.ReaderAt
	Commit() error
	Abort() error
}

// Entry is a committed blob as eviction sees it.
type Entry struct {
	Digest digest.Digest
	Size   int64
	Used   time.Time
}

// TagRecord is what a tag pointed at when it was last asked upstream.
type TagRecord struct {
	Digest    digest.Digest
	MediaType string
	Fetched   time.Time
}
