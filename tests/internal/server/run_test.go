// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package server_test

// Verifies:
//   - limen serves pulls end to end over HTTP, as Docker's registry-mirrors (no ns) and
//     containerd's hosts.toml (ns=) ask them
//   - Three clients pulling one blob at once cost one upstream download
//   - A registry off the list is refused, so the client falls back
//   - A store folder without its marker stops limen from starting
//   - Stopping the context stops limen
//   - A second limen on a store in use is refused
//   - /healthz, /metrics and the prefetch API answer beside the mirror, and so do both setup scripts
//     and what limen knows of the asking client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nauarch/limen/internal/contracts/ops"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/kernel/errkind"
	"github.com/nauarch/limen/internal/server"
	"github.com/nauarch/limen/tests/kit/registry"
)

// start runs limen on a free port with reg standing in for docker.io and ghcr.io, and gives its
// base URL; limen stops when the test ends.
func start(t *testing.T, reg *registry.Registry) string {
	t.Helper()
	return startIn(t, reg, t.TempDir())
}

// startIn is start on a given store folder.
func startIn(t *testing.T, reg *registry.Registry, dir string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan net.Addr, 1)
	done := make(chan error, 1)
	o := server.Options{
		Addr:       "127.0.0.1:0",
		Dir:        dir,
		Init:       true,
		Registries: []string{"docker.io", "ghcr.io"},
		Endpoints:  map[string]string{"docker.io": reg.URL(), "ghcr.io": reg.URL()},
		Listening:  func(a net.Addr) { addr <- a },
		Tools:      fstest.MapFS{"limenctl-linux-amd64": {Data: []byte("linux")}, "limenctl-windows-amd64.exe": {Data: []byte("windows")}},
	}
	go func() { done <- server.Run(ctx, o, slog.New(slog.DiscardHandler)) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("limen stopped with %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("limen did not stop")
		}
	})
	select {
	case a := <-addr:
		return "http://" + a.String()
	case err := <-done:
		t.Fatalf("limen did not start: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("limen did not start")
	}
	return ""
}

// get fetches url and gives its status and body.
func get(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

func TestPullsAreServedEndToEnd(t *testing.T) {
	reg := registry.New(registry.Options{RequireToken: true, RedirectBlobs: true})
	defer reg.Close()
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json"}`)
	reg.PutManifest("vllm/vllm-openai", "v0.32.0", "application/vnd.oci.image.index.v1+json", manifest)
	data := bytes.Repeat([]byte("vllm layer "), 200_000)
	d := reg.PutBlob(data)
	base := start(t, reg)

	if code, body := get(t, base+"/v2/vllm/vllm-openai/manifests/v0.32.0"); code != http.StatusOK || !bytes.Equal(body, manifest) {
		t.Fatalf("manifest, Docker's way: %d %q", code, body)
	}
	if code, _ := get(t, base+"/v2/vllm/vllm-openai/manifests/v0.32.0?ns=ghcr.io"); code != http.StatusOK {
		t.Fatalf("manifest, containerd's way: %d", code)
	}

	release := reg.Hold()
	defer release()
	var pulls sync.WaitGroup
	results := make([][]byte, 3)
	for i := range results {
		pulls.Go(func() { _, results[i] = get(t, base+"/v2/vllm/vllm-openai/blobs/"+d.String()+"?ns=docker.io") })
	}
	time.Sleep(300 * time.Millisecond)
	release()
	pulls.Wait()
	for i, got := range results {
		if !bytes.Equal(got, data) {
			t.Errorf("client %d got %d bytes, want %d", i, len(got), len(data))
		}
	}
	if n := reg.Count(registry.GetBlob); n != 1 {
		t.Errorf("three clients cost %d upstream downloads", n)
	}
}

func TestARegistryOffTheListIsRefused(t *testing.T) {
	reg := registry.New(registry.Options{})
	defer reg.Close()
	base := start(t, reg)
	if code, _ := get(t, base+"/v2/a/b/manifests/latest?ns=quay.io"); code != http.StatusForbidden {
		t.Fatalf("got %d", code)
	}
}

func TestAnUnmarkedStoreStopsLimenFromStarting(t *testing.T) {
	err := server.Run(context.Background(), server.Options{Addr: "127.0.0.1:0", Dir: t.TempDir()}, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("limen started on an unmarked folder")
	}
}

func TestASecondLimenOnAStoreInUseIsRefused(t *testing.T) {
	reg := registry.New(registry.Options{})
	defer reg.Close()
	dir := t.TempDir()
	startIn(t, reg, dir)
	err := server.Run(context.Background(), server.Options{Addr: "127.0.0.1:0", Dir: dir}, slog.New(slog.DiscardHandler))
	if errkind.Of(err) != errkind.Conflict {
		t.Fatalf("got %v, want a conflict", err)
	}
}

func TestOperationsAnswerBesideTheMirror(t *testing.T) {
	reg := registry.New(registry.Options{})
	defer reg.Close()
	layer := reg.PutBlob([]byte("one layer"))
	cfg := reg.PutBlob([]byte("config"))
	reg.PutManifest("library/redis", "8", "application/vnd.oci.image.manifest.v1+json",
		[]byte(`{"schemaVersion":2,"config":{"digest":"`+cfg.String()+`","size":6},"layers":[{"digest":"`+layer.String()+`","size":9}]}`))
	base := start(t, reg)

	if code, body := get(t, base+"/healthz"); code != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("healthz: %d %q", code, body)
	}
	resp, err := http.Post(base+"/api/prefetch", "application/json", strings.NewReader(`{"images":["redis:8"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var job prefetch.Job
	_ = json.NewDecoder(resp.Body).Decode(&job)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || job.ID == "" {
		t.Fatalf("prefetch: %d %+v", resp.StatusCode, job)
	}
	deadline := time.Now().Add(5 * time.Second)
	for job.State == prefetch.Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		_, body := get(t, base+"/api/prefetch/"+job.ID)
		_ = json.Unmarshal(body, &job)
	}
	if job.State != prefetch.Done || job.Fetched != 2 {
		t.Fatalf("job %+v", job)
	}
	if code, _ := get(t, base+"/v2/library/redis/blobs/"+layer.String()); code != http.StatusOK || reg.Count(registry.GetBlob) != 2 {
		t.Errorf("pull after prefetch: %d, upstream blob GETs %d", code, reg.Count(registry.GetBlob))
	}
	_, me := get(t, base+"/api/clients/me")
	var seen ops.Client
	if err := json.Unmarshal(me, &seen); err != nil || seen.Address != "127.0.0.1" || seen.Requests == 0 {
		t.Errorf("clients/me: %s", me)
	}
	_, metrics := get(t, base+"/metrics")
	if !strings.Contains(string(metrics), `limen_blob_requests_total{result="hit"} 1`) {
		t.Errorf("metrics:\n%s", metrics)
	}
}
