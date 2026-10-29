// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package disk_test

// Verifies:
//   - A store held by a live limen refuses a second one, naming the holder
//   - Once released, the store can be taken again
//   - A lock left by a limen that died (stale) is taken over
//   - The holder's heartbeat keeps the lock live
//   - A lock left by a process of this host that is gone, or had this very PID (a restarted
//     container), is taken over at once; a live one, or one from another host, is not

import (
	"strings"
	"testing"
	"time"

	"github.com/nauarch/limen/internal/adapters/storage/disk"
	"github.com/nauarch/limen/internal/kernel/errkind"
	kitclock "github.com/nauarch/limen/tests/kit/clock"
)

func TestALiveLockRefusesASecondLimen(t *testing.T) {
	s, _ := open(t)
	clk := kitclock.At(time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC))
	release, err := s.Lock(disk.LockOptions{Owner: "limen on nas", Clock: clk})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Lock(disk.LockOptions{Owner: "limen on vm", Clock: clk})
	if errkind.Of(err) != errkind.Conflict || !strings.Contains(err.Error(), "limen on nas") {
		t.Fatalf("got %v", err)
	}
	release()
	again, err := s.Lock(disk.LockOptions{Owner: "limen on vm", Clock: clk})
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}

func TestAStaleLockIsTakenOver(t *testing.T) {
	s, _ := open(t)
	clk := kitclock.At(time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC))
	dead, err := s.Lock(disk.LockOptions{Owner: "limen that died", Clock: clk, Beat: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer dead()
	clk.Advance(3 * time.Minute)
	taken, err := s.Lock(disk.LockOptions{Owner: "limen on vm", Clock: clk})
	if err != nil {
		t.Fatalf("a stale lock was not taken over: %v", err)
	}
	taken()
}

func TestTheHeartbeatKeepsTheLockLive(t *testing.T) {
	s, _ := open(t)
	release, err := s.Lock(disk.LockOptions{Owner: "limen on nas", Beat: 20 * time.Millisecond, Stale: 1000 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	time.Sleep(1500 * time.Millisecond)
	if _, err := s.Lock(disk.LockOptions{Owner: "limen on vm", Stale: 1000 * time.Millisecond}); errkind.Of(err) != errkind.Conflict {
		t.Fatalf("a beating lock went stale: %v", err)
	}
}

func TestALockLeftOnThisHostIsTakenOverAtOnce(t *testing.T) {
	clk := kitclock.At(time.Date(2026, 10, 29, 6, 0, 0, 0, time.UTC))
	alive := func(pid int) func(int) bool { return func(p int) bool { return p == pid } }
	for _, c := range []struct {
		name       string
		host       string
		pid, still int
		taken      bool
	}{
		{"holder gone", "limen", 7, 0, true},
		{"same PID again (a restarted container)", "limen", 1, 1, true},
		{"holder still running", "limen", 7, 7, false},
		{"another host", "nas", 7, 0, false},
	} {
		s, _ := open(t)
		left, err := s.Lock(disk.LockOptions{Owner: "limen before", Host: c.host, PID: c.pid, Clock: clk, Beat: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		pid := 1
		if c.pid != 1 {
			pid = 42
		}
		got, err := s.Lock(disk.LockOptions{Owner: "limen now", Host: "limen", PID: pid, Clock: clk, Alive: alive(c.still)})
		if (err == nil) != c.taken {
			t.Errorf("%s: %v", c.name, err)
		}
		if err == nil {
			got()
		}
		left()
	}
}
