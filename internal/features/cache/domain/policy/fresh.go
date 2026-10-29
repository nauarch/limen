// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package policy

import "time"

// Fresh reports whether a tag fetched at fetched may still be answered without asking upstream at
// now: within ttl, and never when ttl is zero or the fetch lies in the future (a clock set back).
func Fresh(fetched, now time.Time, ttl time.Duration) bool {
	age := now.Sub(fetched)
	return ttl > 0 && age >= 0 && age < ttl
}
