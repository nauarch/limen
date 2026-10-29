// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nauarch/limen/internal/contracts/upstream"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/kernel/image"
)

// defaultTimeout bounds connecting and waiting for an answer's headers, not reading its body.
const defaultTimeout = 30 * time.Second

// Options say how registries are reached. Endpoints maps a registry name to its API's base URL
// (scheme and host, such as http://127.0.0.1:5001); any other registry is reached at
// https://<its API host>. Resolver is a DNS server (host:port) to look hosts up with instead of
// the system's, so limen never sees DNS rewrites meant for its clients.
type Options struct {
	Endpoints map[string]string
	Resolver  string
	Timeout   time.Duration
	Clock     clock.Clock
}

// Client reaches the real registries anonymously, fetching pull tokens as they ask for them and
// following blob redirects to their storage (where Go drops the Authorization header, the host
// being another).
type Client struct {
	endpoints map[string]string
	http      *http.Client
	clock     clock.Clock

	mu     sync.Mutex
	tokens map[string]token
}

var _ upstream.Upstream = (*Client)(nil)

// New builds a client.
func New(o Options) *Client {
	timeout := cmp.Or(o.Timeout, defaultTimeout)
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second, Resolver: resolver(o.Resolver)}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
	}
	endpoints := map[string]string{}
	for name, url := range o.Endpoints {
		endpoints[image.Canonical(name)] = strings.TrimSuffix(url, "/")
	}
	clk := o.Clock
	if clk == nil {
		clk = clock.System{}
	}
	return &Client{
		endpoints: endpoints,
		http:      &http.Client{Transport: transport},
		clock:     clk,
		tokens:    map[string]token{},
	}
}

// base is a registry's API base URL.
func (c *Client) base(registry string) string {
	if url, ok := c.endpoints[image.Canonical(registry)]; ok {
		return url
	}
	return "https://" + image.APIHost(registry)
}

// do sends a request about a repository, signing in with a pull token when the registry asks for
// one, and retrying once signed in.
func (c *Client) do(ctx context.Context, method, registry, repository, path string, accept []string) (*http.Response, error) {
	url := c.base(registry) + "/v2/" + repository + path
	resp, err := c.send(ctx, method, url, c.cached(registry, repository), accept)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	discard(resp)
	auth, err := c.authorize(ctx, registry, repository, challenge)
	if err != nil {
		return nil, err
	}
	return c.send(ctx, method, url, auth, accept)
}

// send sends one request; a request that never got an answer is ErrUnreachable.
func (c *Client) send(ctx context.Context, method, url, auth string, accept []string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %w", method, url, errkind.ErrInvalid, err)
	}
	for _, a := range accept {
		req.Header.Add("Accept", a)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s %s: %w", method, url, ctx.Err())
		}
		return nil, fmt.Errorf("%s %s: %w: %w", method, url, errkind.ErrUnreachable, err)
	}
	return resp, nil
}

// resolver looks hosts up at server, or with the system's resolver when server is empty.
func resolver(server string) *net.Resolver {
	if server == "" {
		return nil
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, server)
		},
	}
}
