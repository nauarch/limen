// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"
	"strings"
)

// Unit is limen's systemd service: the binary and the arguments it runs with, the user it runs
// as, and the store folder, the one place it may write.
type Unit struct {
	Binary string
	Args   []string
	User   string
	Dir    string
}

// Render is the unit file. systemd restarts limen whenever it ends, and never gives up doing so;
// limen ends itself when its own /healthz goes unanswered, and systemd's watchdog ends one that
// stopped reporting its checks answered (frozen too hard to end itself). Everything but the
// store is read-only to it.
func Render(u Unit) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Written by limen install.\n")
	fmt.Fprintf(&b, "[Unit]\nDescription=limen, a pull-through cache for container registries\n")
	fmt.Fprintf(&b, "Wants=network-online.target\nAfter=network-online.target\nStartLimitIntervalSec=0\n\n")
	fmt.Fprintf(&b, "[Service]\nType=notify\nNotifyAccess=main\nWatchdogSec=90\nUser=%s\nGroup=%s\n", u.User, u.User)
	fmt.Fprintf(&b, "ExecStart=%s\n", command(append([]string{u.Binary}, u.Args...)))
	fmt.Fprintf(&b, "Restart=always\nRestartSec=2\nLimitNOFILE=65536\n")
	fmt.Fprintf(&b, "NoNewPrivileges=yes\nProtectSystem=strict\nPrivateTmp=yes\n")
	if !underHome(u.Dir) {
		fmt.Fprintf(&b, "ProtectHome=yes\n")
	}
	fmt.Fprintf(&b, "ReadWritePaths=%s\n\n", u.Dir)
	fmt.Fprintf(&b, "[Install]\nWantedBy=multi-user.target\n")
	return []byte(b.String())
}

// Ours reports whether a unit file was written by limen install.
func Ours(content []byte) bool {
	return strings.HasPrefix(string(content), "# Written by limen install.")
}

// command joins a command line as systemd reads it, quoting words with spaces or quotes.
func command(words []string) string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		if strings.ContainsAny(w, " \t\"'\\") {
			w = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(w) + `"`
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}

// underHome reports whether a folder lies where ProtectHome would hide it.
func underHome(dir string) bool {
	for _, home := range []string{"/home", "/root", "/run/user"} {
		if dir == home || strings.HasPrefix(dir, home+"/") {
			return true
		}
	}
	return false
}
