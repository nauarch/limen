// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hooks_test

// Verifies:
//   - A project's stop and start scripts are read by its name; either may be left out
//   - No file is no hooks; a malformed file, or a script not named by absolute path, is invalid

import (
	"testing"

	"github.com/nauarch/limen/internal/features/hosts/domain/hooks"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestHooksAreReadByProject(t *testing.T) {
	got, err := hooks.Parse([]byte(`{"projects":{"dns":{"stop":"/srv/dns/stop.sh","start":"/srv/dns/start.sh"},"web":{"start":"/srv/web/up.sh"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["dns"] != (hooks.Hooks{Stop: "/srv/dns/stop.sh", Start: "/srv/dns/start.sh"}) || got["web"].Stop != "" || got["web"].Start != "/srv/web/up.sh" {
		t.Errorf("%+v", got)
	}
	if none, err := hooks.Parse(nil); err != nil || len(none) != 0 {
		t.Errorf("none: %v %v", none, err)
	}
}

func TestBadHooksAreInvalid(t *testing.T) {
	for _, bad := range []string{`{`, `{"projects":{"web":{"start":"up.sh"}}}`} {
		if _, err := hooks.Parse([]byte(bad)); errkind.Of(err) != errkind.Invalid {
			t.Errorf("%s: %v", bad, err)
		}
	}
}
