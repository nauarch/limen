// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package configure

import (
	"context"
	"path"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/daemonconf"
	"github.com/nauarch/limen/internal/features/hosts/domain/hoststoml"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// PlanUndo works out taking limenctl's configuration off the host: every hosts.toml it wrote goes,
// or, where it replaced one of the owner's (--force), that one comes back from its backup; and the
// limens those files named, with limen when given, leave daemon.json's registry mirrors. Files it
// did not write are left alone. The image store is rollback's business, not this.
func (c *Configurer) PlanUndo(ctx context.Context, limen string) (hostplan.Changes, error) {
	var out hostplan.Changes
	regs, err := c.Files.List(c.Layout.CertsDir)
	if err != nil && errkind.Of(err) != errkind.NotFound {
		return hostplan.Changes{}, err
	}
	mirrors := []string{}
	if limen != "" {
		mirrors = append(mirrors, limen)
	}
	for _, reg := range regs {
		p := c.Layout.HostsToml(reg)
		current, err := c.read(p)
		switch {
		case err != nil:
			return hostplan.Changes{}, err
		case current == nil:
			continue
		case !hoststoml.Ours(current):
			out.Skipped = append(out.Skipped, p+": not written by limenctl")
			continue
		}
		if m := hoststoml.Mirror(current); m != "" && !slices.Contains(mirrors, m) {
			mirrors = append(mirrors, m)
		}
		original, err := c.original(p)
		if err != nil {
			return hostplan.Changes{}, err
		}
		out.Writes = append(out.Writes, hostplan.Write{Path: p, Before: current, After: original})
	}
	before, err := c.read(c.Layout.DaemonJSON)
	if err != nil {
		return hostplan.Changes{}, err
	}
	after, changed, err := daemonconf.WithoutMirrors(before, mirrors)
	if err != nil {
		return hostplan.Changes{}, err
	}
	if changed {
		out.Writes = append(out.Writes, hostplan.Write{Path: c.Layout.DaemonJSON, Before: before, After: after})
		out.Restart = true
	}
	return c.restarting(out), nil
}

// original is the newest backup of path that limenctl did not write itself (what the owner had
// before --force replaced it), nil when there is none.
func (c *Configurer) original(p string) ([]byte, error) {
	names, err := c.Files.List(c.Layout.Backups())
	if errkind.Of(err) == errkind.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	prefix := backupName(p) + "."
	slices.Sort(names)
	for _, name := range slices.Backward(names) {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		data, err := c.Files.Read(path.Join(c.Layout.Backups(), name))
		if err != nil {
			return nil, err
		}
		if !hoststoml.Ours(data) {
			return data, nil
		}
	}
	return nil, nil
}

// backupName is the name a file's backups start with: its path without the leading slash, with
// its slashes (and a Windows drive's colon) as underscores.
func backupName(p string) string {
	return strings.NewReplacer("/", "_", ":", "_").Replace(strings.TrimPrefix(p, "/"))
}
