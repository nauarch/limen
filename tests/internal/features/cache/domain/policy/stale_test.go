// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package policy_test

// Verifies:
//   - A stale tag stands in when upstream is down or answered wrongly
//   - It does not when upstream says the tag is gone, refuses, or the request was bad or canceled

import (
	"context"
	"errors"
	"testing"

	"github.com/nauarch/limen/internal/features/cache/domain/policy"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestAStaleTagStandsInOnlyForAFailingUpstream(t *testing.T) {
	for err, want := range map[error]bool{
		errkind.ErrUnreachable: true,
		errkind.ErrUpstream:    true,
		errors.New("odd"):      true,
		errkind.ErrNotFound:    false,
		errkind.ErrForbidden:   false,
		errkind.ErrInvalid:     false,
		context.Canceled:       false,
	} {
		if got := policy.ServeStale(err); got != want {
			t.Errorf("%v: %v", err, got)
		}
	}
}
