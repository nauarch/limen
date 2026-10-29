// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package dockerd_test

// Verifies:
//   - Once Docker answers, the check's verdict is the answer
//   - Docker never answering is a timeout carrying its last error

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/features/hosts/app/dockerd"
	fake "github.com/nauarch/limen/tests/kit/host"
)

func TestTheCheckDecidesOnceDockerAnswers(t *testing.T) {
	e := &fake.Engine{Current: host.Info{Snapshotter: true}}
	if err := dockerd.Await(context.Background(), e, time.Second, func(i host.Info) error {
		if !i.Snapshotter {
			return errors.New("classic")
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
	if err := dockerd.Await(context.Background(), e, time.Second, func(host.Info) error { return errors.New("no mirror") }); err == nil || err.Error() != "no mirror" {
		t.Errorf("%v", err)
	}
}

func TestDockerNeverAnsweringTimesOut(t *testing.T) {
	e := &fake.Engine{Err: errors.New("cannot connect to the Docker daemon")}
	err := dockerd.Await(context.Background(), e, 50*time.Millisecond, func(host.Info) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "did not come back within 50ms") || !strings.Contains(err.Error(), "cannot connect") {
		t.Errorf("%v", err)
	}
}
