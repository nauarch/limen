// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migration

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
	"github.com/nauarch/limen/internal/kernel/bytesize"
)

// Decide works out a move to the containerd image store from the host's facts and report: what
// refuses it, what to know, which projects stop and start again (through the owner's hooks where
// named) and in what order, what limen fetches ahead, and which images built on the host are
// saved and loaded back. The files it writes are left to the caller.
func Decide(f plan.Facts, r hostplan.Report, req hostplan.MigrateRequest) hostplan.Migration {
	m := hostplan.Migration{To: hostplan.Containerd, Limen: req.Limen, Platform: Platform(f.Engine)}
	if r.Store == hostplan.Containerd {
		m.Refusals = append(m.Refusals, "the host is on the containerd image store already")
	}
	for _, finding := range r.Findings {
		if finding.Level == hostplan.Block {
			m.Refusals = append(m.Refusals, finding.Text)
		}
	}
	if len(r.Loose) > 0 {
		refuseUnlessForced(&m, req.Force, fmt.Sprintf("containers not from compose (%s) stop at the switch and stay in the classic store: limenctl does not recreate them, and only a rollback brings them back", strings.Join(r.Loose, ", ")))
	}
	dnsAtHome := plan.SelfResolving(f.Nameservers, f.Addresses)
	switch {
	case req.Limen == "":
		m.Notes = append(m.Notes, "no limen given: images are pulled from their registries after the switch")
		if dnsAtHome {
			refuseUnlessForced(&m, req.Force, "the host resolves names only through itself, and its DNS is stopped during the switch: give --limen (by IP) so images come from limen")
		}
	case f.LimenErr != "":
		m.Refusals = append(m.Refusals, "limen at "+req.Limen+" cannot be reached: "+f.LimenErr)
	}
	if req.Prune && r.Unused.Count > 0 {
		m.Prune, m.PruneBytes = true, r.Unused.Bytes
	}
	m.Projects, m.Notes = projects(f, r, m.Notes)
	m.Prefetch = prefetch(f, m.Projects)
	keep(f, &m)
	return m
}

// refuseUnlessForced refuses m for why, or, forced, only notes it.
func refuseUnlessForced(m *hostplan.Migration, force bool, why string) {
	if force {
		m.Notes = append(m.Notes, "forced past: "+why)
		return
	}
	m.Refusals = append(m.Refusals, why+" (--force goes ahead anyway)")
}

// projects are the running compose projects in start order (those that may serve the host's DNS
// first), with their owners' hooks; projects not running are noted and left alone.
func projects(f plan.Facts, r hostplan.Report, notes []string) ([]hostplan.MigrateProject, []string) {
	var out []hostplan.MigrateProject
	for _, p := range r.Projects {
		switch {
		case p.Running == 0:
			notes = append(notes, "project "+p.Name+" is not running: it is left alone, and pulls its images afresh when next started")
			continue
		case p.Running < p.Total:
			notes = append(notes, fmt.Sprintf("project %s runs %d of %d containers: starting it again starts all of them", p.Name, p.Running, p.Total))
		}
		h := f.Hooks[p.Name]
		out = append(out, hostplan.MigrateProject{Name: p.Name, Dir: p.Dir, Files: p.Files, Stop: h.Stop, Start: h.Start})
		if h.Stop == "" && len(p.Scripts) > 0 {
			notes = append(notes, "project "+p.Name+" has scripts beside it but no hooks in "+hooks.Path+": Docker compose stops and starts it")
		}
	}
	dns := func(p hostplan.MigrateProject) int {
		if slices.ContainsFunc(f.Containers, func(c host.Container) bool {
			return c.Running && c.Labels[projectLabel] == p.Name && (c.HostNetwork || slices.Contains(c.Ports, 53))
		}) {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(out, func(a, b hostplan.MigrateProject) int { return cmp.Compare(dns(a), dns(b)) })
	return out, notes
}

// prefetch lists the images the projects run that a registry can serve, for limen to fetch ahead.
func prefetch(f plan.Facts, projects []hostplan.MigrateProject) []string {
	pulled := map[string]bool{}
	for _, im := range f.Images {
		pulled[im.ID] = len(im.Digests) > 0
	}
	var refs []string
	for _, c := range f.Containers {
		if pulled[c.ImageID] && slices.ContainsFunc(projects, func(p hostplan.MigrateProject) bool { return p.Name == c.Labels[projectLabel] }) {
			refs = append(refs, c.Image)
		}
	}
	slices.Sort(refs)
	return slices.Compact(refs)
}

// KeepPath is where the images built on the host wait, saved, while the store switches.
const KeepPath = "/var/lib/limenctl/images.tar"

// keep works out the images built on the host, which no registry has: every tagged one, used or
// not (a build's previous image is often kept for going back), saved before the switch and loaded
// after. Their file needs the room on Docker's disk; built images in use with no tag cannot be
// named, so they are noted, as compose builds them again.
func keep(f plan.Facts, m *hostplan.Migration) {
	for _, im := range f.Images {
		if len(im.Digests) > 0 {
			continue
		}
		var tags []string
		for _, t := range im.Tags {
			if t != "<none>:<none>" {
				tags = append(tags, t)
			}
		}
		if len(tags) == 0 {
			if slices.ContainsFunc(f.Containers, func(c host.Container) bool { return c.ImageID == im.ID }) {
				m.Notes = append(m.Notes, "an untagged image built on this host ("+im.ID+") cannot be kept: compose builds it again")
			}
			continue
		}
		m.Keep = append(m.Keep, tags...)
		m.KeepBytes += im.Size
	}
	if len(m.Keep) == 0 {
		return
	}
	slices.Sort(m.Keep)
	m.KeepPath = KeepPath
	if free, ok := f.Free[f.Engine.RootDir]; ok && free >= 0 && free < m.KeepBytes {
		m.Refusals = append(m.Refusals, fmt.Sprintf("keeping the images built on this host needs about %s free for %s; %s is free",
			bytesize.Format(m.KeepBytes), KeepPath, bytesize.Format(free)))
	}
}

// projectLabel is the label Docker compose names a container's project with.
const projectLabel = "com.docker.compose.project"

// Platform is the engine's platform as registries name it (linux/amd64, linux/arm64, linux/arm/v7).
func Platform(e host.Info) string {
	arch := map[string]string{"x86_64": "amd64", "aarch64": "arm64", "armv7l": "arm/v7", "armv6l": "arm/v6", "i686": "386"}[e.Arch]
	if arch == "" {
		arch = e.Arch
	}
	return strings.ToLower(cmp.Or(e.OS, "linux")) + "/" + arch
}
