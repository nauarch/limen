// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Dir is the host's filesystem, seen from Root ("/" on a Linux host; a folder in tests), so every
// absolute path limenctl names lives under it; with no Root, paths are taken as they are (C:/...).
type Dir struct {
	Root string
}

var _ host.Files = Dir{}

// at is where an absolute host path lives under the root.
func (d Dir) at(path string) string { return filepath.Join(d.Root, filepath.FromSlash(path)) }

// Read reads a file.
func (d Dir) Read(path string) ([]byte, error) {
	data, err := os.ReadFile(d.at(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", path, errkind.ErrNotFound)
	}
	return data, err
}

// Write replaces a file through a temporary file beside it and a rename, so a crash never leaves
// it half written.
func (d Dir) Write(path string, data []byte, perm uint32) error {
	full := d.at(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(full), "."+filepath.Base(full)+".*")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr, os.Chmod(f.Name(), fs.FileMode(perm))); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), full); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return nil
}

// List names a folder's entries.
func (d Dir) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(d.at(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", dir, errkind.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

// Free is the space left on the filesystem holding path, looking up through its folders until one
// exists; -1 where the system cannot tell.
func (d Dir) Free(path string) (int64, error) {
	full := d.at(path)
	for {
		if _, err := os.Stat(full); err == nil {
			return free(full)
		}
		parent := filepath.Dir(full)
		if parent == full {
			return -1, nil
		}
		full = parent
	}
}

// Size is the bytes a file or folder holds, every file in it counted once by its size.
func (d Dir) Size(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(d.at(path), func(_ string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Type().IsRegular() {
			info, err := e.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("%s: %w", path, errkind.ErrNotFound)
	}
	return total, err
}

// Remove removes a file or a folder with all it contains; the root itself is never removed.
func (d Dir) Remove(path string) error {
	full := d.at(path)
	if filepath.Clean(full) == filepath.Clean(d.at("/")) {
		return fmt.Errorf("refusing to remove the root %s: %w", d.Root, errkind.ErrForbidden)
	}
	return os.RemoveAll(full)
}

// MakeDir makes a folder and the folders above it.
func (d Dir) MakeDir(path string) error { return os.MkdirAll(d.at(path), 0o755) }
