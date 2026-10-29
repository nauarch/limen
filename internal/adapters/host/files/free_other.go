// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

//go:build !(linux || darwin || freebsd)

package files

// free cannot tell on this system: limenctl's hosts are Linux, and elsewhere it only reads.
func free(string) (int64, error) { return -1, nil }
