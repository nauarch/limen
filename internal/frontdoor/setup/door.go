// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Options are what the door hands out: the limenctl binaries (named limenctl-<os>-<arch>, with
// .exe for Windows, at the top of Files), and where it logs.
type Options struct {
	Files fs.FS
	Log   *slog.Logger
}

// door serves GET /setup, a shell script that runs limenctl on a host without installing it
// (curl -fsSL http://limen:5000/setup | sudo sh -s -- plan), GET /setup.ps1, its PowerShell twin
// for Docker Desktop on Windows (irm http://limen:5000/setup.ps1 | iex), and GET /get/{name}, the
// binaries the scripts fetch.
type door struct {
	o    Options
	once sync.Once
	sums map[string]string
}

// New builds the door.
func New(o Options) http.Handler {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	d := &door{o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /setup", d.script)
	mux.HandleFunc("GET /setup.ps1", d.powershell)
	mux.HandleFunc("GET /get/{name}", d.binary)
	return mux
}

// host is what a Host header may hold to be put into a script: a name or address, and a port.
var host = regexp.MustCompile(`^(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9.-]+)(:[0-9]{1,5})?$`)

// script answers the setup script for limen as the asking host reached it.
func (d *door) script(w http.ResponseWriter, r *http.Request) {
	if !d.plainHost(w, r) {
		return
	}
	sums := map[string]string{}
	for name, sum := range d.checksums() {
		if name != Windows {
			sums[name] = sum
		}
	}
	if len(sums) == 0 {
		http.Error(w, "limen setup: this limen carries no limenctl (built without go generate)", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write(Script("http://"+r.Host, sums))
}

// powershell answers the Windows setup script for limen as the asking machine reached it.
func (d *door) powershell(w http.ResponseWriter, r *http.Request) {
	if !d.plainHost(w, r) {
		return
	}
	sum, ok := d.checksums()[Windows]
	if !ok {
		http.Error(w, "limen setup: this limen carries no limenctl for Windows (built without go generate)", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(PowerShell("http://"+r.Host, sum))
}

// plainHost refuses a Host header that is not a plain host:port, which a script would carry.
func (d *door) plainHost(w http.ResponseWriter, r *http.Request) bool {
	if !host.MatchString(r.Host) {
		http.Error(w, "limen setup: the Host header is not a plain host:port", http.StatusBadRequest)
		return false
	}
	return true
}

// binary answers one limenctl binary.
func (d *door) binary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := d.checksums()[name]; !ok {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(d.o.Files, name)
	if err != nil {
		d.o.Log.Error("reading a binary to hand out", "name", name, "err", err)
		http.Error(w, "limen setup: cannot read "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

// checksums are the SHA-256 of each limenctl binary, by name, worked out once.
func (d *door) checksums() map[string]string {
	d.once.Do(func() {
		d.sums = map[string]string{}
		entries, err := fs.ReadDir(d.o.Files, ".")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "limenctl-") {
				continue
			}
			data, err := fs.ReadFile(d.o.Files, e.Name())
			if err != nil {
				continue
			}
			sum := sha256.Sum256(data)
			d.sums[e.Name()] = hex.EncodeToString(sum[:])
		}
	})
	return d.sums
}

// platforms lists the binaries' platforms (os-arch), sorted.
func platforms(sums map[string]string) []string {
	var out []string
	for name := range sums {
		out = append(out, strings.TrimPrefix(name, "limenctl-"))
	}
	slices.Sort(out)
	return out
}
