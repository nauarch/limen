// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ops_test

// Verifies:
//   - /metrics answers the counters in Prometheus' text format, each name with its HELP and TYPE
//     once and labelled series under it
//   - The build's version is a series of its own, its label escaped

import (
	"net/http"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/contracts/ops"
)

func TestMetricsAreInPrometheusTextFormat(t *testing.T) {
	f := &fake{stats: ops.Stats{BlobHits: 7, BlobJoins: 2, BlobMisses: 3, BytesServed: 1000, BytesFetched: 300, StoreBytes: 5 << 30, MaxBytes: 200 << 30}}
	w := do(f, http.MethodGet, "/metrics", "")
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	body := w.Body.String()
	for _, line := range []string{
		`limen_blob_requests_total{result="hit"} 7`,
		`limen_blob_requests_total{result="joined"} 2`,
		`limen_blob_requests_total{result="miss"} 3`,
		"limen_served_bytes_total 1000",
		"limen_fetched_bytes_total 300",
		"limen_store_bytes 5368709120",
		"# TYPE limen_store_max_bytes gauge",
		`limen_build_info{version="v1.2.3\"x"} 1`,
	} {
		if !strings.Contains(body, line+"\n") {
			t.Errorf("missing %q", line)
		}
	}
	if n := strings.Count(body, "# HELP limen_blob_requests_total "); n != 1 {
		t.Errorf("HELP for blob requests %d times", n)
	}
}
