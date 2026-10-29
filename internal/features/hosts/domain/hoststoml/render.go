// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hoststoml

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/nauarch/limen/internal/kernel/image"
)

// marker begins every file limenctl writes, so it knows which files are its own to replace.
const marker = "# Written by limenctl"

// Render is a registry's hosts.toml: pull through the mirror first, then from the registry
// itself. It sets no dial_timeout: Docker's own resolver dials without reading it, so a mirror
// that is down costs each pull one failed connect however the file says.
func Render(registry, mirror string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s: pulls of %s try limen first, then the registry itself.\n", marker, registry)
	fmt.Fprintf(&b, "server = %q\n", "https://"+image.APIHost(registry))
	fmt.Fprintf(&b, "\n[host.%q]\n  capabilities = [\"pull\", \"resolve\"]\n", strings.TrimSuffix(mirror, "/"))
	return b.Bytes()
}

// Ours reports whether a hosts.toml was written by limenctl.
func Ours(content []byte) bool { return bytes.HasPrefix(content, []byte(marker)) }

// PointsAt reports whether a hosts.toml sends pulls to mirror.
func PointsAt(content []byte, mirror string) bool {
	return bytes.Contains(content, []byte(fmt.Sprintf("[host.%q]", strings.TrimSuffix(mirror, "/"))))
}

// Mirror is the first host a hosts.toml sends pulls to, "" when it names none.
func Mirror(content []byte) string {
	for line := range strings.Lines(string(content)) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "[host.")
		if !ok {
			continue
		}
		if url, err := strconv.Unquote(strings.TrimSuffix(rest, "]")); err == nil {
			return url
		}
	}
	return ""
}
