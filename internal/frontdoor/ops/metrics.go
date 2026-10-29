// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package ops

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// metric is one Prometheus series: its name, help, type, labels (as `key="value"`, or "") and
// value.
type metric struct {
	name, help, kind, labels string
	value                    int64
}

// metrics answers the counters in Prometheus' text format.
func (d *door) metrics(w http.ResponseWriter, _ *http.Request) {
	s := d.o.Reporter.Stats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	write(w, []metric{
		{"limen_build_info", "The version limen was built as.", "gauge", `version="` + label.Replace(d.o.Version) + `"`, 1},
		{"limen_blob_requests_total", "Blob pulls by how they were served.", "counter", `result="hit"`, s.BlobHits},
		{"limen_blob_requests_total", "", "", `result="joined"`, s.BlobJoins},
		{"limen_blob_requests_total", "", "", `result="miss"`, s.BlobMisses},
		{"limen_blob_download_failures_total", "Blob downloads from upstream that failed or stalled.", "counter", "", s.BlobFailures},
		{"limen_served_bytes_total", "Blob bytes sent to clients.", "counter", "", s.BytesServed},
		{"limen_fetched_bytes_total", "Blob bytes fetched from upstream.", "counter", "", s.BytesFetched},
		{"limen_tag_requests_total", "Tag lookups by how they were answered.", "counter", `result="fresh"`, s.TagsFresh},
		{"limen_tag_requests_total", "", "", `result="checked"`, s.TagsChecked},
		{"limen_tag_requests_total", "", "", `result="stale"`, s.TagsStale},
		{"limen_evicted_blobs_total", "Blobs evicted to keep the store under its cap.", "counter", "", s.Evicted},
		{"limen_evicted_bytes_total", "Bytes evicted to keep the store under its cap.", "counter", "", s.EvictedBytes},
		{"limen_store_bytes", "The store's size at the last measurement.", "gauge", "", s.StoreBytes},
		{"limen_store_blobs", "The store's blob count at the last measurement.", "gauge", "", s.StoreBlobs},
		{"limen_store_max_bytes", "The store's cap; 0 when uncapped.", "gauge", "", s.MaxBytes},
	})
}

// label escapes a label value as Prometheus' text format wants it.
var label = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

// write prints series, with HELP and TYPE before the first of each name.
func write(w io.Writer, series []metric) {
	for _, m := range series {
		if m.help != "" {
			fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", m.name, m.help, m.name, m.kind)
		}
		if m.labels != "" {
			fmt.Fprintf(w, "%s{%s} %d\n", m.name, m.labels, m.value)
		} else {
			fmt.Fprintf(w, "%s %d\n", m.name, m.value)
		}
	}
}
