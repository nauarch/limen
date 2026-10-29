// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hooks

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// Path is where a host's owner names the scripts limenctl runs instead of Docker compose.
const Path = "/etc/limenctl.json"

// Hooks are a project's scripts: Stop runs instead of compose stop, Start instead of compose up.
// Either may be "" for compose's own.
type Hooks struct {
	Stop  string `json:"stop,omitempty"`
	Start string `json:"start,omitempty"`
}

// file is /etc/limenctl.json: hooks by compose project name.
type file struct {
	Projects map[string]Hooks `json:"projects"`
}

// Parse reads /etc/limenctl.json; empty is none. Scripts must be named by absolute path.
func Parse(data []byte) (map[string]Hooks, error) {
	if len(data) == 0 {
		return map[string]Hooks{}, nil
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", Path, errkind.ErrInvalid, err)
	}
	for name, h := range f.Projects {
		for _, script := range []string{h.Stop, h.Start} {
			if script != "" && !path.IsAbs(script) {
				return nil, fmt.Errorf("%s: project %s: %q is not an absolute path: %w", Path, name, script, errkind.ErrInvalid)
			}
		}
	}
	if f.Projects == nil {
		f.Projects = map[string]Hooks{}
	}
	return f.Projects, nil
}
