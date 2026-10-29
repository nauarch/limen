// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// token is a pull token and when it stops being used.
type token struct {
	auth    string
	expires time.Time
}

// defaultLifetime is how long a token is taken to last when the registry does not say;
// tokenMargin is how long before its end a token is no longer sent.
const (
	defaultLifetime = 60 * time.Second
	tokenMargin     = 10 * time.Second
)

// cached is the Authorization of a repository's live pull token, or "" when there is none.
func (c *Client) cached(registry, repository string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[registry+"/"+repository]
	if !ok || !c.clock.Now().Before(t.expires) {
		return ""
	}
	return t.auth
}

// authorize answers a 401's Bearer challenge with an anonymous pull token from the realm it names,
// and keeps the token. A realm is only reached over https, unless the registry itself is plain
// http.
func (c *Client) authorize(ctx context.Context, registry, repository, challenge string) (string, error) {
	kind, params := parseChallenge(challenge)
	if !strings.EqualFold(kind, "bearer") {
		return "", fmt.Errorf("registry %s wants a login (%q) for %s: %w", registry, kind, repository, errkind.ErrForbidden)
	}
	realm, err := url.Parse(params["realm"])
	plain := strings.HasPrefix(c.base(registry), "http://")
	if err != nil || realm.Host == "" || !(realm.Scheme == "https" || realm.Scheme == "http" && plain) {
		return "", fmt.Errorf("registry %s names an unusable token address: %w", registry, errkind.ErrUpstream)
	}
	q := realm.Query()
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	q.Set("scope", cmp.Or(params["scope"], "repository:"+repository+":pull"))
	realm.RawQuery = q.Encode()
	resp, err := c.send(ctx, http.MethodGet, realm.String(), "", nil)
	if err != nil {
		return "", err
	}
	defer discard(resp)
	if resp.StatusCode != http.StatusOK {
		return "", failure(resp, "token for "+registry+"/"+repository)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("registry %s sent an unreadable token: %w: %w", registry, errkind.ErrUpstream, err)
	}
	t := cmp.Or(body.Token, body.AccessToken)
	if t == "" {
		return "", fmt.Errorf("registry %s sent no token: %w", registry, errkind.ErrUpstream)
	}
	lifetime := cmp.Or(time.Duration(body.ExpiresIn)*time.Second, defaultLifetime)
	auth := "Bearer " + t
	c.mu.Lock()
	c.tokens[registry+"/"+repository] = token{auth: auth, expires: c.clock.Now().Add(lifetime - tokenMargin)}
	c.mu.Unlock()
	return auth, nil
}

// parseChallenge reads `Bearer realm="…",service="…",scope="…"`.
func parseChallenge(h string) (kind string, params map[string]string) {
	kind, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	params = map[string]string{}
	for rest != "" {
		rest = strings.TrimLeft(rest, " ,")
		key, after, ok := strings.Cut(rest, "=")
		if !ok {
			break
		}
		var val string
		if strings.HasPrefix(after, `"`) {
			end := strings.Index(after[1:], `"`)
			if end < 0 {
				break
			}
			val, rest = after[1:end+1], after[end+2:]
		} else {
			val, rest, _ = strings.Cut(after, ",")
		}
		params[strings.ToLower(strings.TrimSpace(key))] = val
	}
	return kind, params
}
