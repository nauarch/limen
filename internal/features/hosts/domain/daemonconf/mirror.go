// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package daemonconf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nauarch/limen/internal/kernel/errkind"
)

// WithMirror is daemon.json with mirror first among its registry mirrors, every other setting kept.
// changed is false when mirror already led the list. A missing or empty file counts as {}.
// Settings come out sorted by key and indented, as Docker reads them in any order.
func WithMirror(current []byte, mirror string) (out []byte, changed bool, err error) {
	settings, err := decode(current)
	if err != nil {
		return nil, false, err
	}
	mirror = strings.TrimSuffix(mirror, "/")
	var mirrors []any
	if raw, ok := settings["registry-mirrors"]; ok {
		list, ok := raw.([]any)
		if !ok {
			return nil, false, invalid("registry-mirrors is not a list")
		}
		mirrors = list
	}
	if len(mirrors) > 0 && same(mirrors[0], mirror) {
		return current, false, nil
	}
	rest := slices.DeleteFunc(slices.Clone(mirrors), func(m any) bool { return same(m, mirror) })
	settings["registry-mirrors"] = append([]any{mirror}, rest...)
	out, err = encode(settings)
	return out, err == nil, err
}

// same reports whether a listed mirror is url, trailing slash aside.
func same(m any, url string) bool {
	s, ok := m.(string)
	return ok && strings.TrimSuffix(s, "/") == url
}

// decode reads daemon.json's settings; empty is none.
func decode(data []byte) (map[string]any, error) {
	settings := map[string]any{}
	if len(bytes.TrimSpace(data)) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("daemon.json: %w: %w", errkind.ErrInvalid, err)
	}
	return settings, nil
}

// encode writes settings as daemon.json, indented, ending in a newline.
func encode(settings map[string]any) ([]byte, error) {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("daemon.json: %w", err)
	}
	return append(out, '\n'), nil
}

// invalid is a daemon.json that cannot be changed safely, and why.
func invalid(why string) error {
	return fmt.Errorf("daemon.json: %s: %w", why, errkind.ErrInvalid)
}

// WithoutMirrors is daemon.json with the given mirrors taken out of its registry mirrors, every
// other setting kept; an emptied list goes with them. changed is false when none was listed.
func WithoutMirrors(current []byte, gone []string) (out []byte, changed bool, err error) {
	settings, err := decode(current)
	if err != nil {
		return nil, false, err
	}
	raw, ok := settings["registry-mirrors"]
	if !ok {
		return current, false, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, false, invalid("registry-mirrors is not a list")
	}
	kept := slices.DeleteFunc(slices.Clone(list), func(m any) bool {
		return slices.ContainsFunc(gone, func(g string) bool { return same(m, strings.TrimSuffix(g, "/")) })
	})
	if len(kept) == len(list) {
		return current, false, nil
	}
	if len(kept) == 0 {
		delete(settings, "registry-mirrors")
	} else {
		settings["registry-mirrors"] = kept
	}
	out, err = encode(settings)
	return out, err == nil, err
}
