// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package policy

import "github.com/nauarch/limen/internal/kernel/errkind"

// ServeStale reports whether a tag's last known manifest is served in place of a failed upstream
// answer: when upstream is down or answered wrongly, yes; when it says the tag is gone, or the
// request was invalid or canceled, no.
func ServeStale(err error) bool {
	switch errkind.Of(err) {
	case errkind.Upstream, errkind.Unreachable, errkind.Internal:
		return true
	default:
		return false
	}
}
