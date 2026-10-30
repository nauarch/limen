// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/features/hosts/domain/hoststoml"
	"github.com/nauarch/limen/internal/features/hosts/domain/layout"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Gatherer reads a host's facts: from its engine, files, network and compose plugin, and, given
// a limen, from that limen. It only reads.
type Gatherer struct {
	Layout  layout.Layout
	Engine  host.Engine
	Files   host.Files
	Network host.Network
	Compose host.Compose
	Limen   func(url string) host.Limen
}

// Gather reads the facts; limen is the limen to check the host against, "" for none.
func (g *Gatherer) Gather(ctx context.Context, limen string) (plan.Facts, error) {
	info, err := g.Engine.Info(ctx)
	if err != nil {
		return plan.Facts{}, err
	}
	containers, err := g.Engine.Containers(ctx)
	if err != nil {
		return plan.Facts{}, err
	}
	images, err := g.Engine.Images(ctx)
	if err != nil {
		return plan.Facts{}, err
	}
	f := plan.Facts{Engine: info, Containers: containers, Images: images, Limen: limen, Free: map[string]int64{}}
	if data, err := g.Files.Read("/etc/resolv.conf"); err == nil {
		f.Nameservers = plan.ParseResolv(data)
	}
	if addrs, err := g.Network.Addresses(); err == nil {
		f.Addresses = addrs
	}
	if data, err := g.Files.Read(plan.ContainerdConfig); err == nil {
		f.ContainerdConfig = data
	}
	for _, path := range []string{info.RootDir, plan.StoreRoot(info, f.ContainerdConfig)} {
		if path == "" {
			continue
		}
		if n, err := g.Files.Free(path); err == nil {
			f.Free[path] = n
		}
	}
	f.Scripts = g.scripts(containers)
	f.Hooks, f.HooksErr = g.hooks()
	if _, err := g.Compose.Version(ctx); err != nil {
		f.ComposeErr = err.Error()
	}
	if limen != "" {
		g.mirror(ctx, &f)
	}
	return f, nil
}

// scripts lists the shell scripts in each compose project's folder.
func (g *Gatherer) scripts(containers []host.Container) map[string][]string {
	out := map[string][]string{}
	for _, c := range containers {
		project, dir := c.Labels["com.docker.compose.project"], c.Labels["com.docker.compose.project.working_dir"]
		if project == "" || dir == "" {
			continue
		}
		if _, done := out[project]; done {
			continue
		}
		names, err := g.Files.List(dir)
		if err != nil && errkind.Of(err) != errkind.NotFound {
			continue
		}
		var found []string
		for _, n := range names {
			if strings.HasSuffix(n, ".sh") {
				found = append(found, n)
			}
		}
		slices.Sort(found)
		out[project] = found
	}
	return out
}

// mirror checks the host against the limen in f: that limen answers, what it serves, and which
// of those the host's hosts.toml files point at it.
func (g *Gatherer) mirror(ctx context.Context, f *plan.Facts) {
	l := g.Limen(f.Limen)
	if err := l.Healthy(ctx); err != nil {
		f.LimenErr = err.Error()
		return
	}
	regs, err := l.Registries(ctx)
	if err != nil {
		f.LimenErr = err.Error()
		return
	}
	f.Registries = regs
	for _, reg := range regs {
		if data, err := g.Files.Read(g.Layout.HostsToml(reg)); err == nil && hoststoml.PointsAt(data, f.Limen) {
			f.Configured = append(f.Configured, reg)
		}
	}
}

// hooks reads the hooks the host's owner named; none when the file is missing, and why when it
// cannot be read or names a script that cannot run (limenctl runs each as a program of its own).
func (g *Gatherer) hooks() (map[string]hooks.Hooks, string) {
	data, err := g.Files.Read(hooks.Path)
	if errkind.Of(err) == errkind.NotFound {
		return map[string]hooks.Hooks{}, ""
	}
	if err != nil {
		return nil, err.Error()
	}
	owned, err := hooks.Parse(data)
	if err != nil {
		return nil, err.Error()
	}
	names := slices.Sorted(maps.Keys(owned))
	for _, name := range names {
		for _, script := range []string{owned[name].Stop, owned[name].Start} {
			if script == "" {
				continue
			}
			switch ok, err := g.Files.Executable(script); {
			case errkind.Of(err) == errkind.NotFound:
				return nil, fmt.Sprintf("%s: project %s: %s does not exist", hooks.Path, name, script)
			case err != nil:
				return nil, err.Error()
			case !ok:
				return nil, fmt.Sprintf("%s: project %s: %s is not executable (chmod +x %s)", hooks.Path, name, script, script)
			}
		}
	}
	return owned, ""
}
