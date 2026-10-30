// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package files_test

// Verifies:
//   - Host paths live under the root; writing makes the folders, replaces, and leaves no temporary file
//   - A missing file or folder is NotFound
//   - Size counts every file below a folder; Remove takes a folder whole, never the root itself
//   - Free space is asked of the nearest folder that exists, never failing for a missing path
//   - With no root, a full path is taken as it is
//   - A file is executable by its executable bits (Windows has none: every file is)

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/adapters/host/files"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestFilesAreWrittenAndReadUnderTheRoot(t *testing.T) {
	d := files.Dir{Root: t.TempDir()}
	const path = "/etc/docker/certs.d/ghcr.io/hosts.toml"
	for _, data := range []string{"a", "b"} {
		if err := d.Write(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := d.Read(path); err != nil || string(got) != "b" {
		t.Errorf("%q %v", got, err)
	}
	if got, err := d.List("/etc/docker/certs.d/ghcr.io"); err != nil || !slices.Equal(got, []string{"hosts.toml"}) {
		t.Errorf("left %v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(d.Root, "etc", "docker", "certs.d", "ghcr.io", "hosts.toml")); err != nil {
		t.Errorf("not under the root: %v", err)
	}
}

func TestSizeCountsEverythingBelowAndRemoveTakesItAll(t *testing.T) {
	d := files.Dir{Root: t.TempDir()}
	for path, data := range map[string]string{"/var/lib/docker/overlay2/l1/diff/a": "12345", "/var/lib/docker/overlay2/l2/b": "123", "/var/lib/docker/volumes/v/c": "1"} {
		if err := d.Write(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := d.Size("/var/lib/docker/overlay2"); err != nil || n != 8 {
		t.Errorf("size %d %v", n, err)
	}
	if _, err := d.Size("/var/lib/docker/image/overlay2"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing size: %v", err)
	}
	if err := d.Remove("/var/lib/docker/overlay2"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Size("/var/lib/docker/overlay2"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("not removed: %v", err)
	}
	if n, _ := d.Size("/var/lib/docker/volumes"); n != 1 {
		t.Errorf("volumes touched: %d", n)
	}
	if err := d.Remove("/var/lib/docker/overlay2"); err != nil {
		t.Errorf("removing twice: %v", err)
	}
	if err := d.Remove("/"); errkind.Of(err) != errkind.Forbidden {
		t.Errorf("the root: %v", err)
	}
}

func TestMakeDirMakesTheFoldersAbove(t *testing.T) {
	d := files.Dir{Root: t.TempDir()}
	for range 2 {
		if err := d.MakeDir("/var/lib/limenctl"); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := d.List("/var/lib"); err != nil || !slices.Equal(got, []string{"limenctl"}) {
		t.Errorf("%v %v", got, err)
	}
}

func TestMissingIsNotFound(t *testing.T) {
	d := files.Dir{Root: t.TempDir()}
	if _, err := d.Read("/etc/docker/daemon.json"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("read: %v", err)
	}
	if _, err := d.List("/srv/dns"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("list: %v", err)
	}
}

func TestAFileIsExecutableByItsBits(t *testing.T) {
	d := files.Dir{Root: t.TempDir()}
	for path, perm := range map[string]uint32{"/srv/dns/start.sh": 0o755, "/srv/dns/stop.sh": 0o644} {
		if err := d.Write(path, []byte("#!/bin/sh\n"), perm); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := d.Executable("/srv/dns/start.sh"); err != nil || !ok {
		t.Errorf("0755: %v %v", ok, err)
	}
	if ok, err := d.Executable("/srv/dns/stop.sh"); err != nil || ok != (runtime.GOOS == "windows") {
		t.Errorf("0644: %v %v", ok, err)
	}
	if _, err := d.Executable("/srv/dns/update.sh"); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing: %v", err)
	}
}

func TestFreeSpaceIsAskedOfTheNearestFolder(t *testing.T) {
	d := files.Dir{Root: t.TempDir()}
	if n, err := d.Free("/var/lib/containerd"); err != nil || n == 0 || n < -1 {
		t.Errorf("%d %v", n, err)
	}
}

func TestWithNoRootAFullPathIsTakenAsItIs(t *testing.T) {
	full := filepath.ToSlash(filepath.Join(t.TempDir(), ".docker", "certs.d", "ghcr.io", "hosts.toml"))
	if err := (files.Dir{}).Write(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.FromSlash(full)); err != nil || string(got) != "x" {
		t.Errorf("%q %v", got, err)
	}
}
