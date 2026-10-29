// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package daemonconf

// snapshotter is the daemon.json feature that picks the containerd image store.
const snapshotter = "containerd-snapshotter"

// WithContainerdStore is daemon.json with the containerd image store turned on or off, every other
// setting kept. changed is false when the file already said so.
func WithContainerdStore(current []byte, on bool) (out []byte, changed bool, err error) {
	settings, err := decode(current)
	if err != nil {
		return nil, false, err
	}
	features, _ := settings["features"].(map[string]any)
	if features == nil {
		if _, present := settings["features"]; present {
			return nil, false, invalid("features is not an object")
		}
		features = map[string]any{}
	}
	if was, ok := features[snapshotter].(bool); ok && was == on {
		return current, false, nil
	}
	features[snapshotter] = on
	settings["features"] = features
	out, err = encode(settings)
	return out, err == nil, err
}
