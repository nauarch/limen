// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"cmp"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
)

// Compose's labels on the containers it makes.
const (
	labelProject = "com.docker.compose.project"
	labelDir     = "com.docker.compose.project.working_dir"
	labelFiles   = "com.docker.compose.project.config_files"
)

// projects groups the host's compose containers into their projects, sorted by name.
func projects(f Facts) []hostplan.Project {
	byName := map[string]*hostplan.Project{}
	for _, c := range f.Containers {
		name := c.Labels[labelProject]
		if name == "" {
			continue
		}
		p, ok := byName[name]
		if !ok {
			p = &hostplan.Project{Name: name, Dir: c.Labels[labelDir], Scripts: f.Scripts[name]}
			if files := c.Labels[labelFiles]; files != "" {
				p.Files = strings.Split(files, ",")
			}
			byName[name] = p
		}
		p.Total++
		if c.Running {
			p.Running++
		}
	}
	out := make([]hostplan.Project, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b hostplan.Project) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// loose names the containers compose did not make, sorted.
func loose(containers []host.Container) []string {
	var out []string
	for _, c := range containers {
		if c.Labels[labelProject] == "" {
			out = append(out, c.Name)
		}
	}
	slices.Sort(out)
	return out
}
