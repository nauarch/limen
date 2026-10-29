// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package verify_test

// Verifies:
//   - A pull that reaches limen counts its requests, and an image the host lacked is removed again
//   - An image the host had stays; a pull that went around limen counts none
//   - A limen that does not answer, or a pull that fails, is an error

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts/app/verify"
	"github.com/nauarch/limen/internal/kernel/errkind"
	fake "github.com/nauarch/limen/tests/kit/host"
)

// rig is a host whose pulls reach the fake limen, two requests each.
func rig() (*verify.Verifier, *fake.Engine, *fake.Limen) {
	l := &fake.Limen{Seen: 10}
	e := &fake.Engine{}
	e.OnPull = func() { l.Seen += 2 }
	return &verify.Verifier{Engine: e, Limen: l.Client}, e, l
}

func TestAPullThroughLimenIsCounted(t *testing.T) {
	v, e, _ := rig()
	got, err := v.Verify(context.Background(), "http://192.0.2.10:5000")
	if err != nil || got.Requests != 2 || !got.Removed || got.Image != verify.Image {
		t.Fatalf("%+v %v", got, err)
	}
	if !slices.Equal(e.Pulled, []string{verify.Image}) || !slices.Equal(e.RemovedImages, []string{verify.Image}) {
		t.Errorf("pulled %v removed %v", e.Pulled, e.RemovedImages)
	}
}

func TestAnImageTheHostHadStays(t *testing.T) {
	v, e, _ := rig()
	e.ImageList = []host.Image{{ID: "sha256:h", Tags: []string{"hello-world:latest"}}}
	e.OnPull = nil
	got, err := v.Verify(context.Background(), "http://192.0.2.10:5000")
	if err != nil || got.Requests != 0 || got.Removed || len(e.RemovedImages) != 0 {
		t.Errorf("%+v %v %v", got, err, e.RemovedImages)
	}
}

func TestFailuresAreErrors(t *testing.T) {
	v, e, l := rig()
	l.Health = errkind.ErrUpstream
	if _, err := v.Verify(context.Background(), "http://192.0.2.10:5000"); err == nil || len(e.Pulled) != 0 {
		t.Errorf("limen down: %v, pulled %v", err, e.Pulled)
	}
	l.Health, e.Err = nil, errors.New("pull failed")
	if _, err := v.Verify(context.Background(), "http://192.0.2.10:5000"); err == nil {
		t.Error("a failed pull passed")
	}
}
