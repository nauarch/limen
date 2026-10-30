// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package hostplan

import "context"

// Migrator moves a host's images between Docker's image stores: to the containerd one, back to the
// classic one, and clears the classic one once the move is settled. Each is worked out first,
// printed, and made only when asked; progress hears each step as it starts.
type Migrator interface {
	PlanMigration(ctx context.Context, r MigrateRequest) (Migration, error)
	Migrate(ctx context.Context, m Migration, progress func(step string)) error
	PlanRollback(ctx context.Context) (Migration, error)
	Rollback(ctx context.Context, m Migration, progress func(step string)) error
	PlanCleanup(ctx context.Context, force bool) (Cleanup, error)
	Cleanup(ctx context.Context, c Cleanup, progress func(step string)) error
}

// MigrateRequest asks to move the host to the containerd image store: Limen (when given) is
// pointed at and fills ahead of the pulls, Prune removes unused images first, and Force goes ahead
// despite what is only a warning (never despite a block).
type MigrateRequest struct {
	Limen string
	Prune bool
	Force bool
}

// The image stores a migration moves between.
const (
	Containerd = "containerd"
	Classic    = "classic"
)

// Migration is a move between image stores, worked out ahead.
type Migration struct {
	// Refusals say why it cannot go ahead; it goes ahead only when there are none.
	Refusals []string
	// Notes are worth knowing before going ahead.
	Notes []string
	// To is the store the host ends on.
	To string
	// Resume finishes a move that stopped after Docker came back on the new store: only the
	// images saved before it are loaded and the projects started.
	Resume bool
	// Keep are the images built on the host (no registry has them), saved to KeepPath before the
	// switch and loaded after, about KeepBytes (sizes summed per image).
	Keep      []string
	KeepBytes int64
	KeepPath  string
	// Prune removes PruneBytes of unused images first.
	Prune      bool
	PruneBytes int64
	// Limen is the limen asked to fetch Prefetch for Platform (os/arch) before anything stops.
	Limen    string
	Platform string
	Prefetch []string
	// Projects stop in reverse order and start in this order.
	Projects []MigrateProject
	// Writes are the files changed while Docker is down between stop and start.
	Writes []Write
}

// MigrateProject is a compose project a migration stops and starts again: through its hooks,
// the scripts its owner named, when it has them, else through Docker compose.
type MigrateProject struct {
	Name  string
	Dir   string
	Files []string
	Stop  string
	Start string
}

// Cleanup is the classic store's data to remove once the host has moved for good: refused when
// Refusals holds anything. After it, the host cannot roll back.
type Cleanup struct {
	Refusals []string
	Paths    []Removal
}

// Removal is a file or folder to remove and the bytes it frees.
type Removal struct {
	Path  string
	Bytes int64
}
