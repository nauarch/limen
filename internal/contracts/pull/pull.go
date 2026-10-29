// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package pull is the contract between the cache and the front doors that serve pulls: what a
// manifest and a blob are when handed out, and the puller that hands them out.
package pull

import (
	"context"
	"io"

	"github.com/nauarch/limen/internal/kernel/digest"
)

// Manifest is a manifest as served: its media type, its digest and its bytes.
type Manifest struct {
	MediaType string
	Digest    digest.Digest
	Body      []byte
}

// Blob is a blob as served: its size (-1 while unknown) and its bytes. A Body that is also an
// io.ReadSeeker is complete on disk and can answer range requests; one that is not is still
// arriving from upstream.
type Blob struct {
	Size int64
	Body io.ReadCloser
}

// Puller hands out manifests and blobs of the registries it serves.
type Puller interface {
	// Serves reports whether the registry (canonical name, such as docker.io) is served at all.
	Serves(registry string) bool
	// Manifest is a repository's manifest by tag or digest. accept lists the media types the
	// client takes, passed on to upstream.
	Manifest(ctx context.Context, registry, repository, reference string, accept []string) (Manifest, error)
	// Blob is a repository's blob by digest; the caller closes its Body.
	Blob(ctx context.Context, registry, repository string, d digest.Digest) (Blob, error)
}
