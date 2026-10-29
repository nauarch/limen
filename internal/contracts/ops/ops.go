// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

// Package ops is the contract of what limen tells its operators: whether it is healthy and its
// counters, which the ops door serves as /healthz and /metrics.
package ops

import "time"

// Stats are limen's counters since it started, and the store's size now.
type Stats struct {
	// BlobHits were served from the store, BlobJoins joined a running download, BlobMisses
	// started one.
	BlobHits   int64
	BlobJoins  int64
	BlobMisses int64
	// BlobFailures are blob downloads that failed or were stalled.
	BlobFailures int64
	// BytesServed went to clients; BytesFetched came from upstream.
	BytesServed  int64
	BytesFetched int64
	// TagsFresh were answered within the tag lifetime, TagsChecked asked upstream, TagsStale served
	// stale because upstream failed.
	TagsFresh   int64
	TagsChecked int64
	TagsStale   int64
	// Evicted blobs and their bytes.
	Evicted      int64
	EvictedBytes int64
	// StoreBytes and StoreBlobs are the store's size at the last eviction pass; MaxBytes its cap (0
	// when uncapped).
	StoreBytes int64
	StoreBlobs int64
	MaxBytes   int64
}

// Reporter tells limen's health, counters and the registries it serves.
type Reporter interface {
	// Healthy fails when limen cannot serve, such as when its store's share is gone.
	Healthy() error
	Stats() Stats
	// Registries are the registries limen serves, sorted, so hosts can be pointed at each.
	Registries() []string
}

// Client is what limen knows of one client address: the pull requests it made since limen
// started and when it last made one (zero when none), served to that client at GET
// /api/clients/me.
type Client struct {
	Address  string    `json:"address"`
	Requests int64     `json:"requests"`
	Last     time.Time `json:"last"`
}
