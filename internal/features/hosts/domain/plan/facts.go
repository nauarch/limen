// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
)

// Facts are what was read from a host: its engine, containers and images, the nameservers it
// resolves with and its own addresses, the system containerd's settings, free space by path (-1
// unknown), the shell scripts in each compose project's folder and the hooks its owner named (or
// why /etc/limenctl.json could not be read), why docker compose cannot run ("" when it can), and,
// when a limen was given, how the host stands towards it.
type Facts struct {
	Engine           host.Info
	Containers       []host.Container
	Images           []host.Image
	Nameservers      []string
	ContainerdConfig []byte
	Addresses        []string
	Free             map[string]int64
	Scripts          map[string][]string
	Hooks            map[string]hooks.Hooks
	HooksErr         string
	ComposeErr       string
	Limen            string
	LimenErr         string
	Registries       []string
	Configured       []string
}
