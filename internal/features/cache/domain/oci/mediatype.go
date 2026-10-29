// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package oci

// Manifests are the manifest media types limen asks for, indexes first: OCI's and Docker's image
// index and image manifest.
var Manifests = []string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}
