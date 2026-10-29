// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// healthcheckTimeout bounds the whole check, so a hung limen counts as unhealthy.
const healthcheckTimeout = 5 * time.Second

// healthcheck asks a running limen's /healthz (at the URL given, or on the default port) and
// gives the exit code: 0 healthy, 1 not.
func healthcheck(args []string) int {
	url := "http://127.0.0.1:5000/healthz"
	if len(args) > 0 {
		url = args[0]
	}
	c := http.Client{Timeout: healthcheckTimeout}
	resp, err := c.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy:", resp.Status)
		return 1
	}
	return 0
}
