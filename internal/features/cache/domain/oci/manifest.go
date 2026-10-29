// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package oci

import (
	"encoding/json"
	"fmt"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Parsed is what a manifest points at. An index (OCI image index or Docker manifest list) has
// children, one per platform; an image manifest has blobs, its config first and then its layers.
type Parsed struct {
	Children []Child
	Blobs    []Blob
}

// Child is a manifest an index points at, and the platform it is for (empty when the index does
// not say, as for attestations).
type Child struct {
	Digest   digest.Digest
	Platform Platform
}

// Blob is a blob an image manifest points at.
type Blob struct {
	Digest digest.Digest
	Size   int64
}

// raw is every manifest shape at once; which fields are filled tells the kind.
type raw struct {
	Manifests []struct {
		Digest   string    `json:"digest"`
		Platform *Platform `json:"platform"`
	} `json:"manifests"`
	Config *struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"layers"`
}

// Parse reads what a manifest points at. Anything that is neither an index nor an image manifest
// (schema 1, for one) is invalid.
func Parse(body []byte) (Parsed, error) {
	var r raw
	if err := json.Unmarshal(body, &r); err != nil {
		return Parsed{}, fmt.Errorf("manifest: %w: %w", errkind.ErrInvalid, err)
	}
	var out Parsed
	switch {
	case len(r.Manifests) > 0:
		for _, m := range r.Manifests {
			d, err := digest.Parse(m.Digest)
			if err != nil {
				return Parsed{}, err
			}
			c := Child{Digest: d}
			if m.Platform != nil {
				c.Platform = *m.Platform
			}
			out.Children = append(out.Children, c)
		}
	case r.Config != nil:
		d, err := digest.Parse(r.Config.Digest)
		if err != nil {
			return Parsed{}, err
		}
		out.Blobs = append(out.Blobs, Blob{Digest: d, Size: r.Config.Size})
		for _, l := range r.Layers {
			d, err := digest.Parse(l.Digest)
			if err != nil {
				return Parsed{}, err
			}
			out.Blobs = append(out.Blobs, Blob{Digest: d, Size: l.Size})
		}
	default:
		return Parsed{}, fmt.Errorf("manifest: neither an index nor an image manifest: %w", errkind.ErrInvalid)
	}
	return out, nil
}
