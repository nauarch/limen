// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package upstream_test

// Verifies:
//   - A blob is fetched through the registry's redirect to its storage, with its size
//   - A missing blob is not found

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/kernel/digest"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
	"github.com/nauarch/limen/tests/kit/registry"
)

func TestABlobIsFetchedThroughTheRedirect(t *testing.T) {
	reg := registry.New(registry.Options{RequireToken: true, RedirectBlobs: true})
	defer reg.Close()
	data := []byte("a layer")
	d := reg.PutBlob(data)
	c := client(reg, kitclock.At(time.Now()))
	body, size, err := c.Blob(context.Background(), "docker.io", "library/nginx", d)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != string(data) || size != int64(len(data)) {
		t.Fatalf("%q, size %d", got, size)
	}
	if reg.Count(registry.Storage) != 1 {
		t.Errorf("storage asked %d times", reg.Count(registry.Storage))
	}
	if _, _, err := c.Blob(context.Background(), "docker.io", "library/nginx", digest.Of([]byte("none"))); errkind.Of(err) != errkind.NotFound {
		t.Errorf("missing blob: %v", err)
	}
}
