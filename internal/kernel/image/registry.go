// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package image

import "strings"

// Hub is Docker Hub's registry name, the one a mirror request without a namespace is for.
const Hub = "docker.io"

// hubAliases are the other names Docker Hub goes by.
var hubAliases = []string{"index.docker.io", "registry-1.docker.io", "registry.hub.docker.com"}

// Canonical is a registry's name as limen keys it: lower case, Docker Hub's aliases folded into
// Hub, and Hub for an empty name.
func Canonical(registry string) string {
	r := strings.ToLower(strings.TrimSpace(registry))
	if r == "" {
		return Hub
	}
	for _, a := range hubAliases {
		if r == a {
			return Hub
		}
	}
	return r
}

// APIHost is the host a registry's API answers on: Docker Hub's lives apart from its name.
func APIHost(registry string) string {
	if Canonical(registry) == Hub {
		return "registry-1.docker.io"
	}
	return Canonical(registry)
}
