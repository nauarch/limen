// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

//go:build ignore

// build.go builds limenctl for every platform limen hands it out for, into bin/, for dist to
// embed, stamped with LIMEN_VERSION when it is set. go generate runs it from this folder.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// platforms are the ones limenctl is built for.
var platforms = [][2]string{{"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}}

func main() {
	for _, p := range platforms {
		name := fmt.Sprintf("limenctl-%s-%s", p[0], p[1])
		if p[0] == "windows" {
			name += ".exe"
		}
		out, err := filepath.Abs(filepath.Join("bin", name))
		if err != nil {
			fail(err)
		}
		ldflags := "-s -w"
		if v := os.Getenv("LIMEN_VERSION"); v != "" {
			ldflags += " -X main.version=" + v
		}
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags="+ldflags, "-o", out, "github.com/nauarch/limen/cmd/limenctl")
		cmd.Env = append(os.Environ(), "GOOS="+p[0], "GOARCH="+p[1], "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fail(err)
		}
		fmt.Println("built", out)
	}
}

// fail ends the build with why.
func fail(err error) {
	fmt.Fprintln(os.Stderr, "building limenctl:", err)
	os.Exit(1)
}
