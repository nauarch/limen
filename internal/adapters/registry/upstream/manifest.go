// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/nauarch/limen/internal/contracts/pull"
	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// maxManifest is the largest manifest accepted, as registries limit them.
const maxManifest = 4 << 20

// Resolve asks with a HEAD what a tag points at: the digest and media type the registry names.
// An answer without a readable digest is ErrUpstream.
func (c *Client) Resolve(ctx context.Context, registry, repository, tag string, accept []string) (digest.Digest, string, error) {
	what := "manifest " + registry + "/" + repository + ":" + tag
	resp, err := c.do(ctx, http.MethodHead, registry, repository, "/manifests/"+tag, accept)
	if err != nil {
		return "", "", err
	}
	defer discard(resp)
	if resp.StatusCode != http.StatusOK {
		return "", "", failure(resp, what)
	}
	d, err := digest.Parse(resp.Header.Get("Docker-Content-Digest"))
	if err != nil {
		return "", "", fmt.Errorf("%s: no usable digest in the answer: %w", what, errkind.ErrUpstream)
	}
	return d, resp.Header.Get("Content-Type"), nil
}

// Manifest fetches a manifest by tag or digest. When the registry names the digest, the bytes
// must have it.
func (c *Client) Manifest(ctx context.Context, registry, repository, reference string, accept []string) (pull.Manifest, error) {
	what := "manifest " + registry + "/" + repository + "@" + reference
	resp, err := c.do(ctx, http.MethodGet, registry, repository, "/manifests/"+reference, accept)
	if err != nil {
		return pull.Manifest{}, err
	}
	defer discard(resp)
	if resp.StatusCode != http.StatusOK {
		return pull.Manifest{}, failure(resp, what)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifest+1))
	if err != nil {
		return pull.Manifest{}, fmt.Errorf("%s: %w: %w", what, errkind.ErrUnreachable, err)
	}
	if len(body) > maxManifest {
		return pull.Manifest{}, fmt.Errorf("%s: larger than %d bytes: %w", what, maxManifest, errkind.ErrUpstream)
	}
	got := digest.Of(body)
	if named := resp.Header.Get("Docker-Content-Digest"); named != "" && named != got.String() {
		return pull.Manifest{}, fmt.Errorf("%s: content is %s, the registry names %s: %w", what, got, named, errkind.ErrUpstream)
	}
	return pull.Manifest{MediaType: resp.Header.Get("Content-Type"), Digest: got, Body: body}, nil
}
