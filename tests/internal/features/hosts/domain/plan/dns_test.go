// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package plan_test

// Verifies:
//   - resolv.conf's nameservers are read in order, comments and options aside
//   - Resolving only through the host (loopback, or one of its own addresses) warns
//   - The warning names containers publishing port 53 for certain, else those on the host's network
//     as perhaps, else none
//   - A second, outside nameserver, or systemd-resolved's stub, does not warn

import (
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
)

func TestNameserversAreReadInOrder(t *testing.T) {
	got := plan.ParseResolv([]byte("# generated\nnameserver 127.0.0.1\noptions edns0 trust-ad\nnameserver 192.0.2.53\nsearch .\n"))
	if !slices.Equal(got, []string{"127.0.0.1", "192.0.2.53"}) {
		t.Fatalf("%v", got)
	}
}

func TestOnlyResolvingThroughTheHostWarns(t *testing.T) {
	const perhaps = "perhaps served by a container on the host's network (resolver)"
	for _, c := range []struct {
		nameservers []string
		warn        string
	}{
		{[]string{"127.0.0.1"}, perhaps},
		{[]string{"::1"}, perhaps},
		{[]string{"192.0.2.53"}, perhaps},
		{[]string{"127.0.0.1", "192.0.2.53"}, perhaps},
		{[]string{"127.0.0.1", "192.0.2.54"}, ""},
		{[]string{"127.0.0.53"}, ""},
		{[]string{"192.0.2.54", "1.1.1.1"}, ""},
	} {
		f := dnsHost()
		f.Nameservers, f.Addresses = c.nameservers, []string{"127.0.0.1", "192.0.2.53", "fe80::1"}
		r := plan.Decide(f)
		got := has(r, hostplan.Warn, "resolves names only through itself")
		if got != (c.warn != "") || c.warn != "" && !has(r, hostplan.Warn, c.warn) {
			t.Errorf("%v: %+v", c.nameservers, r.Findings)
		}
	}
}

func TestTheLikelyDNSContainerIsNamedHonestly(t *testing.T) {
	f := dnsHost()
	f.Containers = append(f.Containers, host.Container{Name: "unbound", Running: true, Ports: []int{53, 853}})
	if r := plan.Decide(f); !has(r, hostplan.Warn, "served by unbound (publishing port 53)") || has(r, hostplan.Warn, "perhaps") {
		t.Errorf("port 53: %+v", r.Findings)
	}
	f = dnsHost()
	f.Containers = f.Containers[1:]
	if r := plan.Decide(f); !has(r, hostplan.Warn, "by no container limenctl can tell") || has(r, hostplan.Warn, "served by") {
		t.Errorf("without a DNS container: %+v", r.Findings)
	}
}
