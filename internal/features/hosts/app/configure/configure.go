// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package configure

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/app/dockerd"
	"github.com/nauarch/limen/internal/features/hosts/domain/daemonconf"
	"github.com/nauarch/limen/internal/features/hosts/domain/hoststoml"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// RestartWait is how long Docker gets to come back after a restart.
const RestartWait = 60 * time.Second

// Configurer points a host at a limen: a hosts.toml per registry the limen serves, and, on the
// classic image store (which reads hosts.toml for no registry), the limen first among Docker's
// registry mirrors. It works the changes out first; applying them backs up every file it replaces
// and restarts Docker only when it must to read them (see restarting).
type Configurer struct {
	Layout   layout.Layout
	Engine   host.Engine
	Files    host.Files
	Services host.Services
	Limen    func(url string) host.Limen
	Clock    clock.Clock
}

// Plan works out the changes; the limen must answer, since it says which registries it serves.
func (c *Configurer) Plan(ctx context.Context, r hostplan.ConfigureRequest) (hostplan.Changes, error) {
	ch, err := c.plan(ctx, r)
	return c.restarting(ch), err
}

// restarting marks changes Docker reads only when it starts: daemon.json's anywhere, and on
// Docker Desktop, which copies its certs.d into its VM at start, every change.
func (c *Configurer) restarting(ch hostplan.Changes) hostplan.Changes {
	if c.Layout.Desktop && len(ch.Writes) > 0 {
		ch.Restart = true
	}
	return ch
}

// plan works out the changes before restarting has its say.
func (c *Configurer) plan(ctx context.Context, r hostplan.ConfigureRequest) (hostplan.Changes, error) {
	if !strings.HasPrefix(r.Limen, "http://") && !strings.HasPrefix(r.Limen, "https://") {
		return hostplan.Changes{}, fmt.Errorf("limen %q: want http://host:port: %w", r.Limen, errkind.ErrInvalid)
	}
	regs, err := c.Limen(r.Limen).Registries(ctx)
	if err != nil {
		return hostplan.Changes{}, fmt.Errorf("asking limen at %s what it serves: %w", r.Limen, err)
	}
	out := hostplan.Changes{Limen: r.Limen}
	for _, reg := range regs {
		path := c.Layout.HostsToml(reg)
		before, err := c.read(path)
		if err != nil {
			return hostplan.Changes{}, err
		}
		if before != nil && !hoststoml.Ours(before) && !r.Force {
			out.Skipped = append(out.Skipped, path+": not written by limenctl (keep it, or replace it with --force)")
			continue
		}
		after := hoststoml.Render(reg, r.Limen)
		if !bytes.Equal(before, after) {
			out.Writes = append(out.Writes, hostplan.Write{Path: path, Before: before, After: after})
		}
	}
	info, err := c.Engine.Info(ctx)
	if err != nil {
		return hostplan.Changes{}, err
	}
	if info.Snapshotter {
		why := ": the containerd image store reads hosts.toml for every registry, Docker Hub too, so Docker needs no mirror setting"
		if !c.Layout.Desktop {
			why += " and no restart"
		}
		out.Skipped = append(out.Skipped, c.Layout.DaemonJSON+why)
		return out, nil
	}
	before, err := c.read(c.Layout.DaemonJSON)
	if err != nil {
		return hostplan.Changes{}, err
	}
	after, changed, err := daemonconf.WithMirror(before, r.Limen)
	if err != nil {
		return hostplan.Changes{}, err
	}
	if changed {
		out.Writes = append(out.Writes, hostplan.Write{Path: c.Layout.DaemonJSON, Before: before, After: after})
		out.Restart = true
	}
	return out, nil
}

// Apply backs up and writes each file (removing those with nothing after), then, when the changes
// ask, restarts Docker and waits until it answers, with the limen among its mirrors when
// daemon.json was written to point at one.
func (c *Configurer) Apply(ctx context.Context, ch hostplan.Changes) error {
	stamp := c.Clock.Now().UTC().Format("20060102T150405Z")
	for _, w := range ch.Writes {
		if w.Before != nil {
			backup := path.Join(c.Layout.Backups(), backupName(w.Path)+"."+stamp)
			if err := c.Files.Write(backup, w.Before, 0o600); err != nil {
				return fmt.Errorf("backing up %s: %w", w.Path, err)
			}
		}
		if w.After == nil {
			if err := c.remove(w.Path); err != nil {
				return fmt.Errorf("removing %s: %w", w.Path, err)
			}
			continue
		}
		if err := c.Files.Write(w.Path, w.After, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", w.Path, err)
		}
	}
	if !ch.Restart {
		return nil
	}
	if err := c.Services.Restart(ctx, "docker"); err != nil {
		return fmt.Errorf("restarting docker: %w", err)
	}
	mirrored := ch.Limen != "" && slices.ContainsFunc(ch.Writes, func(w hostplan.Write) bool { return w.Path == c.Layout.DaemonJSON && w.After != nil })
	return dockerd.Await(ctx, c.Engine, RestartWait, func(info host.Info) error {
		if !mirrored {
			return nil
		}
		for _, m := range info.Mirrors {
			if strings.TrimSuffix(m, "/") == strings.TrimSuffix(ch.Limen, "/") {
				return nil
			}
		}
		return fmt.Errorf("docker restarted without limen among its mirrors (%v): %w", info.Mirrors, errkind.ErrUpstream)
	})
}

// read reads a file, nil when it does not exist.
func (c *Configurer) read(path string) ([]byte, error) {
	data, err := c.Files.Read(path)
	switch {
	case err == nil:
		return data, nil
	case errkind.Of(err) == errkind.NotFound:
		return nil, nil
	default:
		return nil, err
	}
}

// remove removes a file, and its folder when that is left empty (a registry's certs.d folder
// limenctl made for it).
func (c *Configurer) remove(p string) error {
	if err := c.Files.Remove(p); err != nil {
		return err
	}
	if left, err := c.Files.List(path.Dir(p)); err == nil && len(left) == 0 || errkind.Of(err) == errkind.NotFound {
		return c.Files.Remove(path.Dir(p))
	}
	return nil
}
