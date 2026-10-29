// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream

import (
	"fmt"
	"io"
	"net/http"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// drainLimit is how much of an unwanted body is read so its connection can be reused.
const drainLimit = 64 << 10

// failure names what a registry's error answer means: 404 is ErrNotFound, 401 and 403 are
// ErrForbidden (the content is private), anything else is ErrUpstream.
func failure(resp *http.Response, what string) error {
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%s: %w", what, errkind.ErrNotFound)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s: upstream answered %s: %w", what, resp.Status, errkind.ErrForbidden)
	default:
		return fmt.Errorf("%s: upstream answered %s: %w", what, resp.Status, errkind.ErrUpstream)
	}
}

// discard drains a little of an answer's body and closes it.
func discard(resp *http.Response) {
	_, _ = io.CopyN(io.Discard, resp.Body, drainLimit)
	_ = resp.Body.Close()
}
