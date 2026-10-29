// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"time"

	"github.com/nauarch/limen/internal/frontdoor/setup/dist"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// DefaultRegistries are the public registries served unless others are named.
var DefaultRegistries = []string{
	"docker.io", "ghcr.io", "lscr.io", "quay.io", "registry.k8s.io", "gcr.io",
	"mcr.microsoft.com", "public.ecr.aws", "nvcr.io", "docker.elastic.co", "factory.talos.dev",
}

// Options are how limen runs:
//   - Addr to listen on (port 0 picks a free one); Dir, the store's folder, and Init to make it;
//   - Registries served; Endpoints, base URLs for registries not reached at
//     https://<their API host>; Resolver, the DNS server upstream hosts are looked up with ("" for
//     the system's);
//   - MaxBytes, the store's cap (0 for none); TagTTL, AnswerWithin, StallAfter and EvictEvery,
//     the cache's timing (zero takes its defaults; a negative TagTTL asks upstream every time);
//   - Owner, named in the store's lock, with the Host and PID that let a restarted limen take
//     over a lock its dead predecessor left;
//   - SelfCheck, how often limen asks its own /healthz (zero for 30s, negative never), and Exit,
//     which ends the process when stopping after unanswered checks hangs too (nil for os.Exit);
//   - Notify, which tells systemd limen is ready and, after each answered check, alive (nil for
//     Notify, which does nothing outside systemd);
//   - Tools, the limenctl binaries /setup hands out (nil for those built into limen);
//   - Version, the build, logged at start and shown in /metrics;
//   - Listening, told the address once limen listens.
type Options struct {
	Version      string
	Addr         string
	Dir          string
	Init         bool
	Registries   []string
	Endpoints    map[string]string
	Resolver     string
	MaxBytes     int64
	TagTTL       time.Duration
	AnswerWithin time.Duration
	StallAfter   time.Duration
	EvictEvery   time.Duration
	Owner        string
	Host         string
	PID          int
	SelfCheck    time.Duration
	Exit         func(code int)
	Notify       func(state string) error
	Tools        fs.FS
	Listening    func(net.Addr)
}

// Defaults fills what is left empty and refuses what cannot run.
func Defaults(o Options) (Options, error) {
	if o.Dir == "" {
		return o, fmt.Errorf("no store folder given (--dir): %w", errkind.ErrInvalid)
	}
	if o.MaxBytes < 0 {
		return o, fmt.Errorf("a negative store cap: %w", errkind.ErrInvalid)
	}
	if o.Addr == "" {
		o.Addr = ":5000"
	}
	if len(o.Registries) == 0 {
		o.Registries = DefaultRegistries
	}
	if o.Owner == "" {
		o.Owner = "limen"
	}
	if o.SelfCheck == 0 {
		o.SelfCheck = 30 * time.Second
	}
	if o.Exit == nil {
		o.Exit = os.Exit
	}
	if o.Notify == nil {
		o.Notify = Notify
	}
	if o.Tools == nil {
		tools, err := fs.Sub(dist.Files, "bin")
		if err != nil {
			return o, err
		}
		o.Tools = tools
	}
	return o, nil
}
