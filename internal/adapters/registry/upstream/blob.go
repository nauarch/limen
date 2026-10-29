// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"context"
	"io"
	"net/http"

	"github.com/nauarch/limen/internal/kernel/digest"
)

// Blob starts fetching a blob, following the registry's redirect to its storage. The size is the
// answer's Content-Length, -1 when it names none.
func (c *Client) Blob(ctx context.Context, registry, repository string, d digest.Digest) (io.ReadCloser, int64, error) {
	resp, err := c.do(ctx, http.MethodGet, registry, repository, "/blobs/"+d.String(), nil)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		defer discard(resp)
		return nil, 0, failure(resp, "blob "+registry+"/"+repository+"@"+d.String())
	}
	return resp.Body, resp.ContentLength, nil
}
