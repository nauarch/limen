// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package policy_test

// Verifies:
//   - A tag is fresh within its lifetime and not after it
//   - No lifetime (zero or negative) means never fresh, and so does a fetch in the future

import (
	"testing"
	"time"

	"github.com/nauarch/limen/internal/features/cache/domain/policy"
)

func TestATagIsFreshOnlyWithinItsLifetime(t *testing.T) {
	now := time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		fetched time.Time
		ttl     time.Duration
		want    bool
	}{
		{now.Add(-30 * time.Second), time.Minute, true},
		{now, time.Minute, true},
		{now.Add(-time.Minute), time.Minute, false},
		{now.Add(-time.Second), 0, false},
		{now.Add(-time.Second), -1, false},
		{now.Add(time.Second), time.Minute, false},
	} {
		if got := policy.Fresh(c.fetched, now, c.ttl); got != c.want {
			t.Errorf("fetched %s ago, ttl %s: %v", now.Sub(c.fetched), c.ttl, got)
		}
	}
}
