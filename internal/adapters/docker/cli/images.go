// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"

	"github.com/nauarch/limen/internal/contracts/host"
)

// image is the part of `docker image inspect` limenctl reads.
type image struct {
	ID          string `json:"Id"`
	RepoTags    []string
	RepoDigests []string
	Size        int64
}

// Images reads every local image.
func (c *Client) Images(ctx context.Context) ([]host.Image, error) {
	ids, err := c.ids(ctx, "image", "ls", "--all", "--quiet", "--no-trunc")
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	var raw []image
	if err := c.inspect(ctx, "image", ids, &raw); err != nil {
		return nil, err
	}
	out := make([]host.Image, 0, len(raw))
	for _, r := range raw {
		out = append(out, host.Image{ID: r.ID, Tags: r.RepoTags, Digests: r.RepoDigests, Size: r.Size})
	}
	return out, nil
}

// PruneImages removes every image no container uses.
func (c *Client) PruneImages(ctx context.Context) error {
	_, err := c.run(ctx, "image", "prune", "--all", "--force")
	return err
}

// SaveImages writes the named images to a file at path.
func (c *Client) SaveImages(ctx context.Context, refs []string, path string) error {
	_, err := c.run(ctx, append([]string{"image", "save", "--output", path}, refs...)...)
	return err
}

// LoadImages reads images back from a file SaveImages wrote.
func (c *Client) LoadImages(ctx context.Context, path string) error {
	_, err := c.run(ctx, "image", "load", "--input", path)
	return err
}

// Pull pulls an image, quietly.
func (c *Client) Pull(ctx context.Context, ref string) error {
	_, err := c.run(ctx, "image", "pull", "--quiet", ref)
	return err
}

// RemoveImage removes an image by its reference.
func (c *Client) RemoveImage(ctx context.Context, ref string) error {
	_, err := c.run(ctx, "image", "rm", ref)
	return err
}
