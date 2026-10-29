// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package daemonconf_test

// Verifies:
//   - A missing daemon.json gets the mirror alone
//   - Other settings are kept; the mirror goes first, other mirrors after it, no duplicates
//   - A daemon.json already leading with the mirror is left as it is
//   - Mirrors are taken out, every other setting and mirror kept, an emptied list removed
//   - A malformed daemon.json, or registry-mirrors that is not a list, is invalid

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/features/hosts/domain/daemonconf"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

const mirror = "http://10.0.0.5:5000"

// settings decodes daemon.json.
func settings(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("%s: %v", data, err)
	}
	return s
}

// mirrors lists daemon.json's registry mirrors.
func mirrors(s map[string]any) []string {
	var out []string
	for _, m := range s["registry-mirrors"].([]any) {
		out = append(out, m.(string))
	}
	return out
}

func TestAMissingFileGetsTheMirrorAlone(t *testing.T) {
	out, changed, err := daemonconf.WithMirror(nil, mirror+"/")
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	if got := mirrors(settings(t, out)); !slices.Equal(got, []string{mirror}) || out[len(out)-1] != '\n' {
		t.Errorf("%s", out)
	}
}

func TestOtherSettingsAreKeptAndTheMirrorLeads(t *testing.T) {
	in := []byte(`{"log-driver":"json-file","features":{"containerd-snapshotter":false},"registry-mirrors":["https://other.example",
		"http://10.0.0.5:5000/"]}`)
	out, changed, err := daemonconf.WithMirror(in, mirror)
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	s := settings(t, out)
	if got := mirrors(s); !slices.Equal(got, []string{mirror, "https://other.example"}) {
		t.Errorf("mirrors %v", got)
	}
	if s["log-driver"] != "json-file" || s["features"].(map[string]any)["containerd-snapshotter"] != false {
		t.Errorf("settings lost: %s", out)
	}
	again, changed, err := daemonconf.WithMirror(out, mirror)
	if err != nil || changed || string(again) != string(out) {
		t.Errorf("a second pass changed it: %v %v", changed, err)
	}
}

func TestAMalformedFileIsInvalid(t *testing.T) {
	for _, bad := range []string{`{`, `{"registry-mirrors":"http://x"}`} {
		if _, _, err := daemonconf.WithMirror([]byte(bad), mirror); errkind.Of(err) != errkind.Invalid {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestMirrorsAreTakenOutLeavingTheRest(t *testing.T) {
	in := []byte(`{"log-driver":"local","registry-mirrors":["http://192.0.2.10:5000/","https://other.example"]}`)
	out, changed, err := daemonconf.WithoutMirrors(in, []string{"http://192.0.2.10:5000"})
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	s := settings(t, out)
	if got := mirrors(s); !slices.Equal(got, []string{"https://other.example"}) || s["log-driver"] != "local" {
		t.Errorf("%s", out)
	}
	emptied, _, _ := daemonconf.WithoutMirrors(out, []string{"https://other.example/"})
	if _, still := settings(t, emptied)["registry-mirrors"]; still {
		t.Errorf("an empty list was kept: %s", emptied)
	}
	if same, changed, err := daemonconf.WithoutMirrors(in, []string{"http://elsewhere"}); err != nil || changed || string(same) != string(in) {
		t.Errorf("not listed: %v %v", changed, err)
	}
}
