// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package network_test

// Verifies:
//   - The host's addresses are read from its interfaces, as plain IPs, loopback among them

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/adapters/host/network"
)

func TestTheHostsAddressesAreRead(t *testing.T) {
	addrs, err := network.Interfaces{}.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addrs {
		if _, err := netip.ParseAddr(a); err != nil {
			t.Errorf("%q is not a plain IP", a)
		}
	}
	if !slices.ContainsFunc(addrs, func(a string) bool { return netip.MustParseAddr(a).IsLoopback() }) {
		t.Errorf("no loopback in %v", addrs)
	}
}
