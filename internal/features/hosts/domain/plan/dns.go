// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan

import (
	"net/netip"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
)

// resolvedStubs are systemd-resolved's local addresses: local, but not a DNS server of the host's
// own making.
var resolvedStubs = []string{"127.0.0.53", "127.0.0.54"}

// ParseResolv lists the nameservers of a resolv.conf, in order.
func ParseResolv(data []byte) []string {
	var out []string
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			out = append(out, fields[1])
		}
	}
	return out
}

// dnsFindings warn when the host resolves only through itself (a loopback address, or one of its
// own): then a migration that stops its containers may stop its DNS too, and with it every pull
// that needs a name. The containers that may serve it are named: those publishing port 53 for
// certain, else those on the host's network, any of which might.
func dnsFindings(nameservers, own []string, containers []host.Container) []hostplan.Finding {
	if !SelfResolving(nameservers, own) {
		return nil
	}
	lead := "the host resolves names only through itself (" + strings.Join(nameservers, ", ") + ")"
	advice := ": stopping it stops the host's DNS. Point limen by IP, or add a second nameserver (another DNS server, then a public one)"
	if s := running(containers, func(c host.Container) bool { return slices.Contains(c.Ports, 53) }); len(s) > 0 {
		return []hostplan.Finding{{Level: hostplan.Warn, Text: lead + ", served by " + strings.Join(s, ", ") + " (publishing port 53)" + advice}}
	}
	if s := running(containers, func(c host.Container) bool { return c.HostNetwork }); len(s) > 0 {
		return []hostplan.Finding{{Level: hostplan.Warn, Text: lead + ", perhaps served by a container on the host's network (" + strings.Join(s, ", ") + ")" + advice}}
	}
	return []hostplan.Finding{{Level: hostplan.Warn, Text: lead + ", by no container limenctl can tell: add a second nameserver so a local DNS outage does not stop pulls"}}
}

// running names the running containers that match, sorted.
func running(containers []host.Container, match func(host.Container) bool) []string {
	var out []string
	for _, c := range containers {
		if c.Running && match(c) {
			out = append(out, c.Name)
		}
	}
	slices.Sort(out)
	return out
}

// ownServer reports whether a nameserver is the host itself: a loopback address other than
// systemd-resolved's stubs, or one of the host's own addresses.
func ownServer(ns string, own []string) bool {
	addr, err := netip.ParseAddr(ns)
	if err != nil {
		return false
	}
	if addr.IsLoopback() {
		return !slices.Contains(resolvedStubs, ns)
	}
	return slices.ContainsFunc(own, func(a string) bool { o, err := netip.ParseAddr(a); return err == nil && o == addr })
}

// SelfResolving reports whether the host resolves names only through itself: every nameserver a
// loopback address (systemd-resolved's stubs aside) or one of the host's own addresses.
func SelfResolving(nameservers, own []string) bool {
	return len(nameservers) > 0 && !slices.ContainsFunc(nameservers, func(ns string) bool { return !ownServer(ns, own) })
}
