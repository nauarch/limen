// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/features/cache"
	"github.com/nauarch/limen/internal/kernel/bytesize"
	"github.com/nauarch/limen/internal/server"
)

// parse reads the command line into the server's options.
func parse(args []string) (server.Options, error) {
	o := server.Options{Endpoints: map[string]string{}}
	set := flag.NewFlagSet("limen", flag.ContinueOnError)
	set.StringVar(&o.Addr, "addr", ":5000", "the address to listen on; port 0 picks a free one")
	set.StringVar(&o.Dir, "dir", "", "the store's folder, on a local disk or a mounted share (required)")
	set.BoolVar(&o.Init, "init", false, "make the store's folder and its marker file when they are missing")
	set.StringVar(&o.Resolver, "resolver", "", "a DNS server (host:port) to look up registries with, instead of the system's")
	set.Func("max-size", "the store's cap, such as 200G or 1.5T; least recently used blobs are evicted beyond it (default none)", func(s string) error {
		n, err := bytesize.Parse(s)
		o.MaxBytes = n
		return err
	})
	set.DurationVar(&o.TagTTL, "tag-ttl", cache.DefaultTagTTL, "how long a checked tag is answered without asking upstream; negative asks every time")
	set.DurationVar(&o.AnswerWithin, "answer-within", cache.DefaultAnswerWithin, "how long a pull waits for upstream before failing over (keep it under the clients' ~30s)")
	set.DurationVar(&o.StallAfter, "stall-after", cache.DefaultStallAfter, "how long a download may receive nothing before it is given up")
	set.DurationVar(&o.EvictEvery, "evict-every", cache.DefaultEvictEvery, "how often the store is measured and trimmed to its cap")
	set.DurationVar(&o.SelfCheck, "self-check", 30*time.Second, "how often limen asks its own /healthz, exiting after 3 unanswered in a row so its service manager restarts it (0 never)")
	set.Func("registries", "comma-separated registries to serve (default "+strings.Join(server.DefaultRegistries, ",")+")", func(s string) error {
		for r := range strings.SplitSeq(s, ",") {
			if r = strings.TrimSpace(r); r != "" {
				o.Registries = append(o.Registries, r)
			}
		}
		return nil
	})
	set.Func("upstream", "name=url: reach a registry at url instead of https://<its API host> (repeatable)", func(s string) error {
		name, url, ok := strings.Cut(s, "=")
		if !ok || name == "" || url == "" {
			return errors.New("want name=url")
		}
		o.Endpoints[name] = url
		return nil
	})
	if err := set.Parse(args); err != nil {
		return o, err
	}
	if o.SelfCheck == 0 {
		o.SelfCheck = -1
	}
	host, _ := os.Hostname()
	o.Host, o.PID = host, os.Getpid()
	o.Owner = fmt.Sprintf("limen on %s (pid %d)", host, o.PID)
	return o, nil
}
