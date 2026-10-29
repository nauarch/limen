// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package upstream is the contract of the real registries the cache pulls from. An HTTP adapter
// implements it.
package upstream

import (
	"context"
	"io"

	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/kernel/digest"
)

// Upstream is the real registries. Failures wrap errkind: ErrNotFound for what upstream does not
// have, ErrUpstream for a wrong answer, ErrUnreachable when it cannot be reached.
type Upstream interface {
	// Resolve asks what a tag points at without fetching the manifest (a HEAD, which registries
	// such as Docker Hub do not count against pull limits).
	Resolve(ctx context.Context, registry, repository, tag string, accept []string) (digest.Digest, string, error)
	// Manifest fetches a manifest by tag or digest.
	Manifest(ctx context.Context, registry, repository, reference string, accept []string) (pull.Manifest, error)
	// Blob starts fetching a blob, following redirects; size is -1 when upstream does not say.
	Blob(ctx context.Context, registry, repository string, d digest.Digest) (body io.ReadCloser, size int64, err error)
}
