// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"net"
	"net/netip"

	"github.com/nauarch/limen/internal/contracts/host"
)

// Interfaces is the host's network, read from its interfaces.
type Interfaces struct{}

var _ host.Network = Interfaces{}

// Addresses lists the IP address of every interface, without its prefix length.
func (Interfaces) Addresses() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr().String())
		}
	}
	return out, nil
}
