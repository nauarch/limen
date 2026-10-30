// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/kernel/bytesize"
)

// Decide turns a host's facts into its report: the store, projects, images, disk, mirror state,
// and findings ordered most serious first.
func Decide(f Facts) hostplan.Report {
	r := hostplan.Report{
		Version:  f.Engine.Version,
		Store:    "classic",
		Projects: projects(f),
		Loose:    loose(f.Containers),
		Mirror: hostplan.Mirror{
			Limen: f.Limen, Unreachable: f.LimenErr, Registries: f.Registries,
			Configured: f.Configured, Daemon: f.Engine.Mirrors,
		},
	}
	if f.Engine.Snapshotter {
		r.Store = "containerd"
	}
	r.InUse, r.Unused = images(f)
	for _, path := range []string{f.Engine.RootDir, StoreRoot(f.Engine, f.ContainerdConfig)} {
		if path != "" {
			r.Disk = append(r.Disk, hostplan.Disk{Path: path, Free: freeAt(f.Free, path)})
		}
	}
	var out []hostplan.Finding
	out = append(out, storeFindings(f, r)...)
	out = append(out, dnsFindings(f.Nameservers, f.Addresses, f.Containers)...)
	if !f.Engine.Snapshotter {
		out = append(out, projectFindings(r.Projects, r.Loose, f)...)
		out = append(out, writtenFindings(f.Containers)...)
	}
	out = append(out, mirrorFindings(f)...)
	slices.SortStableFunc(out, func(a, b hostplan.Finding) int { return cmp.Compare(rank(a.Level), rank(b.Level)) })
	r.Findings = out
	return r
}

// rank orders levels most serious first.
func rank(level string) int {
	switch level {
	case hostplan.Block:
		return 0
	case hostplan.Warn:
		return 1
	default:
		return 2
	}
}

// freeAt is the free space recorded for a path, -1 when unknown.
func freeAt(free map[string]int64, path string) int64 {
	if n, ok := free[path]; ok {
		return n
	}
	return -1
}

// images splits the host's images into those a container uses and the rest (what a prune frees).
func images(f Facts) (inUse, unused hostplan.Images) {
	used := map[string]bool{}
	for _, c := range f.Containers {
		used[c.ImageID] = true
	}
	for _, im := range f.Images {
		if used[im.ID] {
			inUse.Count++
			inUse.Bytes += im.Size
		} else {
			unused.Count++
			unused.Bytes += im.Size
		}
	}
	return inUse, unused
}

// storeFindings are about the image store and what a migration to the containerd one would need.
func storeFindings(f Facts, r hostplan.Report) []hostplan.Finding {
	var out []hostplan.Finding
	if f.Engine.Swarm {
		out = append(out, hostplan.Finding{Level: hostplan.Block, Text: "swarm mode is on: limenctl does not migrate swarm hosts"})
	}
	if f.Engine.UsernsRemap {
		out = append(out, hostplan.Finding{Level: hostplan.Block, Text: "userns-remap is on: Docker turns the containerd image store off while it is"})
	}
	if f.Engine.Snapshotter {
		out = append(out, hostplan.Finding{Level: hostplan.Info, Text: "containerd image store: every registry can be pulled through limen"})
		return out
	}
	out = append(out, hostplan.Finding{Level: hostplan.Info, Text: "classic image store: only Docker Hub pulls go through limen until the host is migrated (limenctl migrate)"})
	need, root := 2*r.InUse.Bytes, StoreRoot(f.Engine, f.ContainerdConfig)
	if free := freeAt(f.Free, root); free >= 0 && free < need {
		out = append(out, hostplan.Finding{Level: hostplan.Block, Text: fmt.Sprintf(
			"migrating needs about %s free on %s (the in-use images again, packed and unpacked); %s is free",
			bytesize.Format(need), root, bytesize.Format(free))})
	}
	if r.Unused.Count > 0 {
		out = append(out, hostplan.Finding{Level: hostplan.Info, Text: fmt.Sprintf(
			"%d images no container uses (up to %s) can be pruned before migrating (--prune)", r.Unused.Count, bytesize.Format(r.Unused.Bytes))})
	}
	return out
}

// projectFindings are about containers a migration could not bring back, scripts it would not
// know to run, the hooks it would run instead of compose, and a compose it could not run.
func projectFindings(projects []hostplan.Project, loose []string, f Facts) []hostplan.Finding {
	var out []hostplan.Finding
	owned := f.Hooks
	if f.HooksErr != "" {
		out = append(out, hostplan.Finding{Level: hostplan.Warn, Text: "hooks cannot be used: " + f.HooksErr})
	}
	if f.ComposeErr != "" && slices.ContainsFunc(projects, func(p hostplan.Project) bool {
		return p.Running > 0 && (owned[p.Name].Stop == "" || owned[p.Name].Start == "")
	}) {
		out = append(out, hostplan.Finding{Level: hostplan.Block, Text: "docker compose (v2) cannot run (" + f.ComposeErr + "): a migration stops and starts projects with it; install the compose plugin, or name both hooks of every running project in " + hooks.Path})
	}
	if len(loose) > 0 {
		out = append(out, hostplan.Finding{Level: hostplan.Warn, Text: fmt.Sprintf(
			"%d containers are not from compose (%s): a migration stops them with the old store and does not recreate them (only a rollback brings them back), so it refuses unless forced",
			len(loose), strings.Join(loose, ", "))})
	}
	for _, p := range projects {
		h, hooked := owned[p.Name]
		switch {
		case hooked:
			out = append(out, hostplan.Finding{Level: hostplan.Info, Text: fmt.Sprintf(
				"project %s stops through %s and starts through %s (%s)", p.Name, cmp.Or(h.Stop, "docker compose stop"), cmp.Or(h.Start, "docker compose up"), hooks.Path)})
		case len(p.Scripts) > 0:
			out = append(out, hostplan.Finding{Level: hostplan.Info, Text: fmt.Sprintf(
				"project %s has scripts beside it (%s): if they must run around stop and start, name them as its hooks in %s",
				p.Name, strings.Join(p.Scripts, ", "), hooks.Path)})
		}
	}
	return out
}

// mirrorFindings are about how the host stands towards the limen asked about.
func mirrorFindings(f Facts) []hostplan.Finding {
	switch {
	case f.Limen == "":
		return []hostplan.Finding{{Level: hostplan.Info, Text: "no limen given (--limen): the mirror setup was not checked"}}
	case f.LimenErr != "":
		return []hostplan.Finding{{Level: hostplan.Warn, Text: "limen at " + f.Limen + " cannot be reached: " + f.LimenErr}}
	}
	var missing []string
	for _, reg := range f.Registries {
		if !slices.Contains(f.Configured, reg) {
			missing = append(missing, reg)
		}
	}
	var out []hostplan.Finding
	if len(missing) > 0 {
		out = append(out, hostplan.Finding{Level: hostplan.Warn, Text: "not pointed at limen yet for " + strings.Join(missing, ", ") + ": run limenctl configure"})
	}
	if !f.Engine.Snapshotter && !slices.ContainsFunc(f.Engine.Mirrors, func(m string) bool { return strings.TrimSuffix(m, "/") == strings.TrimSuffix(f.Limen, "/") }) {
		out = append(out, hostplan.Finding{Level: hostplan.Warn, Text: "Docker's registry-mirrors does not name limen: run limenctl configure"})
	}
	if len(out) == 0 {
		out = append(out, hostplan.Finding{Level: hostplan.Info, Text: "pointed at limen for every registry it serves"})
	}
	return out
}

// keptWrites is how much a container may have written outside its volumes before the report says
// so: below it are pid files, caches and the like.
const keptWrites = 1 << 20

// writtenFindings name the containers holding data outside their volumes: the new image store
// starts them afresh, so that data stays behind in the classic one.
func writtenFindings(containers []host.Container) []hostplan.Finding {
	var named []string
	for _, c := range containers {
		if c.Written >= keptWrites {
			named = append(named, fmt.Sprintf("%s %s", c.Name, bytesize.Format(c.Written)))
		}
	}
	if len(named) == 0 {
		return nil
	}
	slices.Sort(named)
	return []hostplan.Finding{{Level: hostplan.Warn, Text: "containers hold data outside their volumes (" + strings.Join(named, ", ") +
		"): after a migration they start without it; it stays in the classic store until cleanup"}}
}
