// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/features/hosts/domain/migration"
	"github.com/nauarch/limen/internal/kernel/bytesize"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// PlanCleanup works out the classic store's data to remove, with the bytes each frees; data
// already gone is left out.
func (m *Migrator) PlanCleanup(ctx context.Context, force bool) (hostplan.Cleanup, error) {
	if m.Configurer.Layout.Desktop {
		return hostplan.Cleanup{Refusals: []string{onDesktop}}, nil
	}
	info, err := m.Engine.Info(ctx)
	if err != nil {
		return hostplan.Cleanup{}, err
	}
	containers, err := m.Engine.Containers(ctx)
	if err != nil {
		return hostplan.Cleanup{}, err
	}
	known := make([]string, 0, len(containers))
	for _, c := range containers {
		known = append(known, c.ID)
	}
	dirs, err := m.Files.List(path.Join(info.RootDir, "containers"))
	if err != nil && errkind.Of(err) != errkind.NotFound {
		return hostplan.Cleanup{}, err
	}
	j, err := m.journal()
	if err != nil {
		return hostplan.Cleanup{}, err
	}
	c := migration.Cleanup(info, j, known, dirs, force)
	kept := c.Paths[:0]
	for _, p := range c.Paths {
		n, err := m.Files.Size(p.Path)
		switch {
		case errkind.Of(err) == errkind.NotFound:
			continue
		case err != nil:
			return hostplan.Cleanup{}, err
		}
		p.Bytes = n
		kept = append(kept, p)
	}
	c.Paths = kept
	return c, nil
}

// Cleanup removes the classic store's data and records that there is no way back.
func (m *Migrator) Cleanup(ctx context.Context, c hostplan.Cleanup, progress func(string)) error {
	if len(c.Refusals) > 0 {
		return fmt.Errorf("the cleanup is refused: %s: %w", strings.Join(c.Refusals, "; "), errkind.ErrConflict)
	}
	for _, p := range c.Paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		progress(fmt.Sprintf("removing %s (%s)", p.Path, bytesize.Format(p.Bytes)))
		if err := m.Files.Remove(p.Path); err != nil {
			return fmt.Errorf("removing %s: %w", p.Path, err)
		}
	}
	j, err := m.journal()
	if err != nil {
		return err
	}
	if j == nil {
		j = &migration.Journal{Started: m.Clock.Now().UTC().Format(time.RFC3339)}
	}
	j.State = migration.Cleaned
	return m.record(*j)
}
