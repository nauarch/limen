// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"fmt"
	"strings"
)

// Script is the setup script for the limen at base: it fetches the limenctl built for the host,
// checks it against sums (SHA-256 by binary name), runs it with the arguments the script was
// given (plan when none; --limen base added to the commands that take it), and removes it.
func Script(base string, sums map[string]string) []byte {
	var cases strings.Builder
	for _, p := range platforms(sums) {
		fmt.Fprintf(&cases, "  %s) sum=%s ;;\n", p, sums["limenctl-"+p])
	}
	return []byte(fmt.Sprintf(`#!/bin/sh
# limen setup: runs limenctl from the limen at %[1]s without installing it.
#   curl -fsSL %[1]s/setup | sudo sh -s -- plan
#   curl -fsSL %[1]s/setup | sudo sh -s -- configure --yes
# limen: https://github.com/nauarch/limen, Copyright 2026 darki73, Apache-2.0.
set -eu
limen='%[1]s'
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "limen setup: no limenctl for $(uname -m)" >&2; exit 1 ;;
esac
platform="$(uname -s | tr '[:upper:]' '[:lower:]')-$arch"
case "$platform" in
%[2]s  *) echo "limen setup: this limen carries no limenctl for $platform" >&2; exit 1 ;;
esac
[ "$(id -u)" = 0 ] || { echo "limen setup: run it as root (| sudo sh)" >&2; exit 1; }
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "$dir/limenctl" "$limen/get/limenctl-$platform"
else
  wget -qO "$dir/limenctl" "$limen/get/limenctl-$platform"
fi
echo "$sum  $dir/limenctl" | sha256sum -c - >/dev/null 2>&1 || { echo "limen setup: the downloaded limenctl does not match its checksum" >&2; exit 1; }
chmod 755 "$dir/limenctl"
[ $# -gt 0 ] || set -- plan
command=$1
shift
case "$command" in
  plan|configure|migrate|verify) set -- "$command" --limen "$limen" "$@" ;;
  *) set -- "$command" "$@" ;;
esac
"$dir/limenctl" "$@"
`, base, cases.String()))
}
