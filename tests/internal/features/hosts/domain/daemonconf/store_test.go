// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package daemonconf_test

// Verifies:
//   - The containerd image store is turned on or off in features, every other setting kept
//   - A file already saying so is left as it is
//   - features that is not an object is invalid

import (
	"testing"

	"github.com/nauarch/limen/internal/features/hosts/domain/daemonconf"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

func TestTheStoreIsTurnedOnAndOff(t *testing.T) {
	in := []byte(`{"registry-mirrors":["http://10.0.0.5:5000"],"features":{"buildkit":true}}`)
	on, changed, err := daemonconf.WithContainerdStore(in, true)
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	s := settings(t, on)
	f := s["features"].(map[string]any)
	if f["containerd-snapshotter"] != true || f["buildkit"] != true || len(mirrors(s)) != 1 {
		t.Errorf("%s", on)
	}
	if again, changed, _ := daemonconf.WithContainerdStore(on, true); changed || string(again) != string(on) {
		t.Errorf("turned on twice: %s", again)
	}
	off, changed, err := daemonconf.WithContainerdStore(on, false)
	if err != nil || !changed || settings(t, off)["features"].(map[string]any)["containerd-snapshotter"] != false {
		t.Errorf("off: %s %v", off, err)
	}
	if fresh, _, _ := daemonconf.WithContainerdStore(nil, true); settings(t, fresh)["features"].(map[string]any)["containerd-snapshotter"] != true {
		t.Errorf("fresh: %s", fresh)
	}
}

func TestFeaturesThatIsNotAnObjectIsInvalid(t *testing.T) {
	if _, _, err := daemonconf.WithContainerdStore([]byte(`{"features":["x"]}`), true); errkind.Of(err) != errkind.Invalid {
		t.Errorf("%v", err)
	}
}
