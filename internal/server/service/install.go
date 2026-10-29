// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Where limen install puts things.
const (
	// Binary is where limen is installed.
	Binary = "/usr/local/bin/limen"
	// UnitPath is limen's systemd unit.
	UnitPath = "/etc/systemd/system/limen.service"
	// User is the system user limen runs as.
	User = "limen"
	// marker says a folder is limen's store (the store's own Marker, repeated to keep this
	// package free of the store).
	marker = ".limen-volume"
)

// Runner runs a command and gives what it printed.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Installer installs limen as a systemd service on this host, or removes it. Root is where the
// host's filesystem is seen from ("/" on a host, a folder in tests); Self is the running binary,
// copied into place; Healthy asks the installed limen whether it answers.
type Installer struct {
	Root    string
	Self    string
	Run     Runner
	Healthy func(ctx context.Context) error
}

// Step is one thing an install or uninstall does: what it says, and the doing.
type Step struct {
	Say string
	Do  func(ctx context.Context) error
}

// Install works out installing limen to serve with args (its own flags, --dir among them) from
// the store in dir. A folder holding files but no marker is refused: it is not limen's.
func (in Installer) Install(ctx context.Context, dir string, args []string) ([]Step, error) {
	if !path.IsAbs(dir) {
		return nil, fmt.Errorf("--dir %q: want an absolute path: %w", dir, errkind.ErrInvalid)
	}
	if _, err := os.Stat(in.at("/run/systemd/system")); err != nil {
		return nil, fmt.Errorf("this host does not run systemd; run limen in a container instead: %w", errkind.ErrInvalid)
	}
	var steps []Step
	if same, _ := sameFile(in.Self, in.at(Binary)); !same {
		steps = append(steps, Step{"copy " + in.Self + " to " + Binary, func(context.Context) error { return in.copySelf() }})
	}
	if _, err := in.Run(ctx, "id", "-u", User); err != nil {
		steps = append(steps, Step{"create the system user " + User, func(ctx context.Context) error {
			_, err := in.Run(ctx, "useradd", "--system", "--no-create-home", "--home-dir", dir, "--shell", "/usr/sbin/nologin", User)
			return err
		}})
	}
	store, err := in.store(dir)
	if err != nil {
		return nil, err
	}
	steps = append(steps, store...)
	unit := Render(Unit{Binary: Binary, Args: args, User: User, Dir: dir})
	current, err := os.ReadFile(in.at(UnitPath))
	switch {
	case err == nil && !Ours(current):
		return nil, fmt.Errorf("%s exists and was not written by limen install; remove it first: %w", UnitPath, errkind.ErrConflict)
	case !bytes.Equal(current, unit):
		steps = append(steps, Step{"write " + UnitPath + ":\n" + indent(unit), func(context.Context) error {
			return write(in.at(UnitPath), unit, 0o644)
		}})
	}
	steps = append(steps,
		Step{"enable limen and (re)start it", func(ctx context.Context) error {
			for _, args := range [][]string{{"daemon-reload"}, {"enable", "limen"}, {"restart", "limen"}} {
				if _, err := in.Run(ctx, "systemctl", args...); err != nil {
					return err
				}
			}
			return nil
		}},
		Step{"wait until limen answers /healthz", in.await},
	)
	return steps, nil
}

// Uninstall works out removing the service and the binary. The store and the user stay: the
// store is a cache anyone can delete, and a user may own files elsewhere.
func (in Installer) Uninstall(ctx context.Context) ([]Step, error) {
	current, err := os.ReadFile(in.at(UnitPath))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	case !Ours(current):
		return nil, fmt.Errorf("%s was not written by limen install; leaving it: %w", UnitPath, errkind.ErrConflict)
	}
	return []Step{
		{"stop and disable limen", func(ctx context.Context) error {
			_, err := in.Run(ctx, "systemctl", "disable", "--now", "limen")
			return err
		}},
		{"remove " + UnitPath + " and " + Binary, func(ctx context.Context) error {
			for _, p := range []string{UnitPath, Binary} {
				if err := os.Remove(in.at(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			_, err := in.Run(ctx, "systemctl", "daemon-reload")
			return err
		}},
	}, nil
}

// store works out the store folder: made and marked when missing or empty, used as it is when
// marked, refused when it holds anything else.
func (in Installer) store(dir string) ([]Step, error) {
	full := in.at(dir)
	if _, err := os.Stat(filepath.Join(full, marker)); err == nil {
		return []Step{{"use " + dir + ", limen's store already, owned by " + User, func(ctx context.Context) error {
			_, err := in.Run(ctx, "chown", User+":"+User, dir)
			return err
		}}}, nil
	}
	entries, err := os.ReadDir(full)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(entries) > 0 {
		return nil, fmt.Errorf("%s holds files but no %s, so it is not limen's store; pick another --dir: %w", dir, marker, errkind.ErrConflict)
	}
	return []Step{{"make " + dir + " limen's store, owned by " + User, func(ctx context.Context) error {
		if err := os.MkdirAll(full, 0o750); err != nil {
			return err
		}
		if err := write(filepath.Join(full, marker), nil, 0o640); err != nil {
			return err
		}
		_, err := in.Run(ctx, "chown", "-R", User+":"+User, dir)
		return err
	}}}, nil
}

// await waits up to a minute for the installed limen to answer.
func (in Installer) await(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for {
		err := in.Healthy(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("limen did not answer within a minute (journalctl -u limen says why): %w", err)
		case <-time.After(time.Second):
		}
	}
}

// copySelf copies the running binary into place through a temporary file and a rename, which also
// replaces a binary that is running.
func (in Installer) copySelf() error {
	data, err := os.ReadFile(in.Self)
	if err != nil {
		return err
	}
	return write(in.at(Binary), data, 0o755)
}

// at is where a host path lives under the root.
func (in Installer) at(p string) string { return filepath.Join(in.Root, filepath.FromSlash(p)) }

// write replaces a file whole through a temporary file beside it.
func write(p string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".new"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// sameFile reports whether two paths are the one file.
func sameFile(a, b string) (bool, error) {
	ia, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ia, ib), nil
}

// indent indents a file for printing under its step.
func indent(data []byte) string {
	return "    " + string(bytes.ReplaceAll(bytes.TrimRight(data, "\n"), []byte("\n"), []byte("\n    ")))
}
