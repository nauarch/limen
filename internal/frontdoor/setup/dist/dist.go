// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package dist carries the limenctl binaries limen hands out at /get, built into it at compile
// time. `go generate ./internal/frontdoor/setup/dist` builds them into bin/ first; a limen built
// without them serves no limenctl and says so.
package dist

import "embed"

//go:generate go run build.go

// Files are the binaries, under bin/, named limenctl-<os>-<arch>, with .exe for Windows.
//
//go:embed bin
var Files embed.FS
