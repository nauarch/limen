// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin || freebsd

package files

import "syscall"

// free is the space left for unprivileged use on the filesystem holding an existing path.
func free(path string) (int64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return -1, err
	}
	return int64(s.Bavail) * int64(s.Bsize), nil
}
