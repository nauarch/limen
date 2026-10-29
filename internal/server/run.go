// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/nauarch/limen/internal/adapters/registry/upstream"
	"github.com/nauarch/limen/internal/adapters/storage/disk"
	"github.com/nauarch/limen/internal/features/cache"
	"github.com/nauarch/limen/internal/frontdoor/clients"
	"github.com/nauarch/limen/internal/frontdoor/mirror"
	"github.com/nauarch/limen/internal/frontdoor/ops"
	"github.com/nauarch/limen/internal/frontdoor/setup"
	"github.com/nauarch/limen/internal/kernel/bytesize"
)

// shutdownGrace is how long running answers get to finish once limen is told to stop.
const shutdownGrace = 10 * time.Second

// upstreamTimeout bounds connecting to a registry and waiting for its answer's headers; it is
// kept under the cache's answer deadline so a slow registry fails over in time.
const upstreamTimeout = 10 * time.Second

// Run composes limen (the store and its lock, the upstream client, the cache and its upkeep, the
// mirror and ops doors) and serves until ctx ends. Downloads run on ctx too, so they stop with it.
func Run(ctx context.Context, o Options, log *slog.Logger) error {
	o, err := Defaults(o)
	if err != nil {
		return err
	}
	store, err := disk.Open(disk.Options{Root: o.Dir, Init: o.Init})
	if err != nil {
		return err
	}
	release, err := store.Lock(disk.LockOptions{Owner: o.Owner, Host: o.Host, PID: o.PID})
	if err != nil {
		return err
	}
	defer release()
	// On the way out: stop the upkeep and downloads, wait for the upkeep, then release the lock.
	var upkeep sync.WaitGroup
	defer upkeep.Wait()
	ctx, stopAll := context.WithCancel(ctx)
	defer stopAll()
	up := upstream.New(upstream.Options{Endpoints: o.Endpoints, Resolver: o.Resolver, Timeout: upstreamTimeout})
	c, err := cache.Compose(cache.Options{
		Store: store, Upstream: up, Registries: o.Registries, MaxBytes: o.MaxBytes,
		TagTTL: o.TagTTL, AnswerWithin: o.AnswerWithin, StallAfter: o.StallAfter, EvictEvery: o.EvictEvery,
		Background: ctx, Log: log,
	})
	if err != nil {
		return err
	}
	upkeep.Go(func() { c.Maintain(ctx) })

	mux := http.NewServeMux()
	book := clients.New(0, nil)
	mux.Handle("/v2/", book.Note(mirror.New(mirror.Options{Puller: c, Log: log})))
	mux.HandleFunc("GET /api/clients/me", book.Me)
	door := setup.New(setup.Options{Files: o.Tools, Log: log})
	for _, p := range []string{"/setup", "/setup.ps1", "/get/"} {
		mux.Handle(p, door)
	}
	mux.Handle("/", ops.New(ops.Options{Reporter: c, Prefetcher: c, Version: o.Version, Log: log}))
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", o.Addr, err)
	}
	log.Info("limen serving", "version", o.Version, "addr", ln.Addr().String(), "store", o.Dir, "cap", capOf(o.MaxBytes), "registries", o.Registries)
	if o.Listening != nil {
		o.Listening(ln.Addr())
	}
	_ = o.Notify("READY=1")
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	unanswered := make(chan error, 1)
	if o.SelfCheck > 0 {
		go func() {
			unanswered <- Watch(ctx, ln.Addr(), o.SelfCheck, func() { _ = o.Notify("WATCHDOG=1") }, log)
		}()
	}
	select {
	case err := <-served:
		return err
	case err := <-unanswered:
		if err == nil {
			break
		}
		// Stopping may hang on what hangs limen: give it the grace, then end the process.
		time.AfterFunc(shutdownGrace, func() {
			log.Error("limen did not stop in time after its self-check; exiting", "err", err)
			o.Exit(1)
		})
		stopAll()
		release()
		_ = srv.Close()
		return err
	case <-ctx.Done():
	}
	stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(stop); err != nil {
		return fmt.Errorf("shutting down: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// capOf spells the store's cap for the log.
func capOf(max int64) string {
	if max == 0 {
		return "none"
	}
	return bytesize.Format(max)
}
