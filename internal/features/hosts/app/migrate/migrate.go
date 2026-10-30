// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/contracts/host"
	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/contracts/prefetch"
	"github.com/nauarch/limen/internal/features/hosts/app/configure"
	"github.com/nauarch/limen/internal/features/hosts/app/dockerd"
	"github.com/nauarch/limen/internal/features/hosts/app/report"
	"github.com/nauarch/limen/internal/features/hosts/domain/daemonconf"
	"github.com/nauarch/limen/internal/features/hosts/domain/migration"
	"github.com/nauarch/limen/internal/features/hosts/domain/plan"
	"github.com/nauarch/limen/internal/kernel/clock"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// The defaults of waiting during a migration.
const (
	// PrefetchWait is how long limen gets to fetch every image ahead.
	PrefetchWait = 30 * time.Minute
	// PrefetchPoll is how often limen is asked how the prefetch is going.
	PrefetchPoll = 2 * time.Second
)

// Migrator moves a host between Docker's image stores. It reads the host through the gatherer,
// writes files (with backups) through the configurer, and records how far it got in a journal,
// so a failed move can be rolled back.
type Migrator struct {
	Gatherer   *report.Gatherer
	Configurer *configure.Configurer
	Engine     host.Engine
	Files      host.Files
	Services   host.Services
	Compose    host.Compose
	Scripts    host.Scripts
	Limen      func(url string) host.Limen
	Clock      clock.Clock
	// PrefetchWait bounds the prefetch and PrefetchPoll paces asking about it; zero is the
	// package's default of each.
	PrefetchWait time.Duration
	PrefetchPoll time.Duration
}

var _ hostplan.Migrator = (*Migrator)(nil)

// onDesktop refuses a move between stores on Docker Desktop, which makes it itself.
const onDesktop = "Docker Desktop switches image stores in its own settings (General: Use containerd for pulling and storing images); limenctl only points it at limen"

// PlanMigration works out the move to the containerd store, with the files it writes: daemon.json
// turning the store on, and, given a limen, everything limenctl configure would write.
func (m *Migrator) PlanMigration(ctx context.Context, r hostplan.MigrateRequest) (hostplan.Migration, error) {
	if m.Configurer.Layout.Desktop {
		return hostplan.Migration{To: hostplan.Containerd, Refusals: []string{onDesktop}}, nil
	}
	facts, err := m.Gatherer.Gather(ctx, r.Limen)
	if err != nil {
		return hostplan.Migration{}, err
	}
	if facts.HooksErr != "" {
		return hostplan.Migration{}, fmt.Errorf("%s: %w", facts.HooksErr, errkind.ErrInvalid)
	}
	j, err := m.journal()
	if err != nil {
		return hostplan.Migration{}, err
	}
	if j != nil && j.State == migration.Switched && facts.Engine.Snapshotter {
		return m.resumption(j), nil
	}
	out := migration.Decide(facts, plan.Decide(facts), r)
	if j != nil && (j.State == migration.Stopping || j.State == migration.Switched) {
		out.Refusals = append(out.Refusals, "a migration stopped half way (state "+j.State+"): run limenctl rollback first")
	}
	if len(out.Refusals) > 0 {
		return out, nil
	}
	var writes []hostplan.Write
	if r.Limen != "" {
		ch, err := m.Configurer.Plan(ctx, hostplan.ConfigureRequest{Limen: r.Limen, Force: r.Force})
		if err != nil {
			return hostplan.Migration{}, err
		}
		writes = ch.Writes
		out.Notes = append(out.Notes, ch.Skipped...)
	}
	i := slices.IndexFunc(writes, func(w hostplan.Write) bool { return w.Path == m.Configurer.Layout.DaemonJSON })
	if i < 0 {
		current, err := m.read(m.Configurer.Layout.DaemonJSON)
		if err != nil {
			return hostplan.Migration{}, err
		}
		writes, i = append(writes, hostplan.Write{Path: m.Configurer.Layout.DaemonJSON, Before: current, After: current}), len(writes)
	}
	if writes[i].After, _, err = daemonconf.WithContainerdStore(writes[i].After, true); err != nil {
		return hostplan.Migration{}, err
	}
	out.Writes = writes
	return out, nil
}

// resumption finishes the move the journal recorded: Docker is on the containerd store already,
// so what is left is loading the images saved before the switch (when they still wait) and
// starting the projects.
func (m *Migrator) resumption(j *migration.Journal) hostplan.Migration {
	out := hostplan.Migration{To: hostplan.Containerd, Resume: true, Projects: j.Projects,
		Notes: []string{"a migration started " + j.Started + " stopped after Docker came back on the containerd store: this finishes it (limenctl rollback still goes back)"}}
	if _, err := m.Files.Size(migration.KeepPath); err == nil {
		out.Keep, out.KeepPath = []string{migration.KeepPath}, migration.KeepPath
	}
	return out
}

// Migrate makes the move: save the images built on the host, prune, have limen fetch ahead, stop
// the projects, write the files, restart Docker on the containerd store, load the saved images,
// and pull and start the projects again. Up to the stop nothing is changed; past it, a failure
// leaves the journal saying how far it got, for limenctl rollback, or, once Docker is on the new
// store, for migrate to finish (a resumption).
func (m *Migrator) Migrate(ctx context.Context, mig hostplan.Migration, progress func(string)) error {
	if len(mig.Refusals) > 0 {
		return fmt.Errorf("the migration is refused: %s: %w", strings.Join(mig.Refusals, "; "), errkind.ErrConflict)
	}
	if mig.Resume {
		return m.finish(ctx, mig, progress)
	}
	if len(mig.Keep) > 0 {
		progress(fmt.Sprintf("saving %d images built on this host to %s", len(mig.Keep), mig.KeepPath))
		if err := m.Files.MakeDir(path.Dir(mig.KeepPath)); err != nil {
			return fmt.Errorf("making %s: %w (nothing was stopped)", path.Dir(mig.KeepPath), err)
		}
		if err := m.Engine.SaveImages(ctx, mig.Keep, mig.KeepPath); err != nil {
			return fmt.Errorf("saving the images built on this host: %w (nothing was stopped)", err)
		}
	}
	if mig.Prune {
		progress("removing images no container uses")
		if err := m.Engine.PruneImages(ctx); err != nil {
			return fmt.Errorf("pruning: %w", err)
		}
	}
	if mig.Limen != "" && len(mig.Prefetch) > 0 {
		progress(fmt.Sprintf("limen fetches %d images for %s ahead", len(mig.Prefetch), mig.Platform))
		if err := m.prefetch(ctx, mig); err != nil {
			return fmt.Errorf("%w (nothing was stopped)", err)
		}
	}
	daemon, err := m.read(m.Configurer.Layout.DaemonJSON)
	if err != nil {
		return err
	}
	j := migration.Journal{State: migration.Stopping, Started: m.Clock.Now().UTC().Format(time.RFC3339), Projects: mig.Projects, Daemon: daemon}
	if err := m.record(j); err != nil {
		return err
	}
	if err := m.stop(ctx, mig.Projects, false, progress); err != nil {
		return err
	}
	if err := m.switchStore(ctx, mig, true, progress); err != nil {
		return err
	}
	j.State = migration.Switched
	if err := m.record(j); err != nil {
		return err
	}
	return m.finish(ctx, mig, progress)
}

// finish is a move's part on the new store: load the saved images, pull and start the projects,
// and record the move made.
func (m *Migrator) finish(ctx context.Context, mig hostplan.Migration, progress func(string)) error {
	if len(mig.Keep) > 0 {
		progress("loading the images built on this host into the new store")
		if err := m.Engine.LoadImages(ctx, mig.KeepPath); err != nil {
			return fmt.Errorf("loading %s: %w (limenctl rollback goes back to the classic store, where they still are)", mig.KeepPath, err)
		}
		if err := m.Files.Remove(mig.KeepPath); err != nil {
			return fmt.Errorf("removing %s: %w", mig.KeepPath, err)
		}
	}
	if err := m.start(ctx, mig.Projects, true, progress); err != nil {
		return err
	}
	j, err := m.journal()
	if err != nil || j == nil {
		return cmp.Or(err, fmt.Errorf("the migration's journal is gone: %w", errkind.ErrNotFound))
	}
	j.State = migration.Migrated
	return m.record(*j)
}

// PlanRollback works out the way back to the classic store from the journal.
func (m *Migrator) PlanRollback(context.Context) (hostplan.Migration, error) {
	if m.Configurer.Layout.Desktop {
		return hostplan.Migration{To: hostplan.Classic, Refusals: []string{onDesktop}}, nil
	}
	j, err := m.journal()
	if err != nil {
		return hostplan.Migration{}, err
	}
	daemon, err := m.read(m.Configurer.Layout.DaemonJSON)
	if err != nil {
		return hostplan.Migration{}, err
	}
	return migration.Rollback(j, m.Configurer.Layout.DaemonJSON, daemon), nil
}

// Rollback goes back: stop the projects, put daemon.json back, restart Docker on the classic
// store, and start the projects again, where their old containers wait.
func (m *Migrator) Rollback(ctx context.Context, back hostplan.Migration, progress func(string)) error {
	if len(back.Refusals) > 0 {
		return fmt.Errorf("the rollback is refused: %s: %w", strings.Join(back.Refusals, "; "), errkind.ErrConflict)
	}
	j, err := m.journal()
	if err != nil || j == nil {
		return cmp.Or(err, fmt.Errorf("no migration is recorded: %w", errkind.ErrNotFound))
	}
	if err := m.stop(ctx, back.Projects, true, progress); err != nil {
		return err
	}
	if err := m.switchStore(ctx, back, false, progress); err != nil {
		return err
	}
	if err := m.start(ctx, back.Projects, false, progress); err != nil {
		return err
	}
	j.State = migration.RolledBack
	return m.record(*j)
}

// stop stops the projects, last first. Going back, a project that cannot be stopped (Docker may
// be down: that is often why) is reported and passed, as restarting Docker stops it anyway; going
// forward it ends the move, since nothing has changed yet.
func (m *Migrator) stop(ctx context.Context, projects []hostplan.MigrateProject, back bool, progress func(string)) error {
	for _, p := range slices.Backward(projects) {
		progress("stopping " + p.Name)
		var err error
		if p.Stop != "" {
			err = m.Scripts.Run(ctx, p.Stop, p.Dir)
		} else {
			err = m.Compose.Stop(ctx, project(p))
		}
		switch {
		case err != nil && back:
			progress(fmt.Sprintf("could not stop %s (%v): going on, restarting Docker stops it", p.Name, err))
		case err != nil:
			return fmt.Errorf("stopping %s: %w (limenctl rollback starts what was stopped)", p.Name, err)
		}
	}
	return nil
}

// start starts the projects in order. With pull, each project's images are pulled just before it
// starts, while its stop hook's arrangements still hold: a DNS project's stop hook often points
// the host at another resolver, and a pull on the containerd store asks the registry itself for
// more than limen serves (an image's referrers), which needs the host to resolve names. Its start
// hook then finds the images there. A failed pull is passed: starting pulls again.
func (m *Migrator) start(ctx context.Context, projects []hostplan.MigrateProject, pull bool, progress func(string)) error {
	for _, p := range projects {
		if pull {
			progress("pulling the images of " + p.Name)
			if err := m.Compose.Pull(ctx, project(p)); err != nil {
				progress(fmt.Sprintf("could not pull the images of %s (%v): starting it pulls them", p.Name, err))
			}
		}
		progress("starting " + p.Name)
		var err error
		if p.Start != "" {
			err = m.Scripts.Run(ctx, p.Start, p.Dir)
		} else {
			err = m.Compose.Up(ctx, project(p))
		}
		if err != nil {
			return fmt.Errorf("starting %s: %w (limenctl rollback goes back to the classic store)", p.Name, err)
		}
	}
	return nil
}

// switchStore writes the files, restarts Docker, and checks it came back on the wanted store.
func (m *Migrator) switchStore(ctx context.Context, mig hostplan.Migration, containerd bool, progress func(string)) error {
	progress("writing " + strings.Join(paths(mig.Writes), ", "))
	if err := m.Configurer.Apply(ctx, hostplan.Changes{Writes: mig.Writes, Limen: mig.Limen}); err != nil {
		return err
	}
	progress("restarting Docker on the " + mig.To + " image store")
	if err := m.Services.Restart(ctx, "docker"); err != nil {
		return fmt.Errorf("restarting docker: %w (limenctl rollback puts daemon.json back)", err)
	}
	return dockerd.Await(ctx, m.Engine, configure.RestartWait, func(info host.Info) error {
		if info.Snapshotter != containerd {
			return fmt.Errorf("docker came back on the wrong image store (wanted %s; see journalctl -u docker): %w", mig.To, errkind.ErrUpstream)
		}
		return nil
	})
}

// prefetch has limen fetch the images and waits until it has; any it could not fetch fails it.
func (m *Migrator) prefetch(ctx context.Context, mig hostplan.Migration) error {
	l := m.Limen(mig.Limen)
	job, err := l.Prefetch(ctx, prefetch.Request{Images: mig.Prefetch, Platforms: []string{mig.Platform}})
	if err != nil {
		return fmt.Errorf("asking limen to fetch ahead: %w", err)
	}
	wait := cmp.Or(m.PrefetchWait, PrefetchWait)
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for job.State == prefetch.Running {
		select {
		case <-ctx.Done():
			return fmt.Errorf("limen did not fetch ahead within %s: %w", wait, errkind.ErrUnreachable)
		case <-time.After(cmp.Or(m.PrefetchPoll, PrefetchPoll)):
		}
		if job, err = l.Job(ctx, job.ID); err != nil {
			return fmt.Errorf("asking limen how the prefetch goes: %w", err)
		}
	}
	if job.State != prefetch.Done {
		return fmt.Errorf("limen could not fetch %s: %w", strings.Join(job.Failures, "; "), errkind.ErrUpstream)
	}
	return nil
}

// journal reads the journal; nil when there is none.
func (m *Migrator) journal() (*migration.Journal, error) {
	data, err := m.read(migration.JournalPath)
	if err != nil || data == nil {
		return nil, err
	}
	j, err := migration.Decode(data)
	return &j, err
}

// record writes the journal.
func (m *Migrator) record(j migration.Journal) error {
	if err := m.Files.Write(migration.JournalPath, migration.Encode(j), 0o600); err != nil {
		return fmt.Errorf("recording the migration: %w", err)
	}
	return nil
}

// read reads a file, nil when it does not exist.
func (m *Migrator) read(path string) ([]byte, error) {
	data, err := m.Files.Read(path)
	if errkind.Of(err) == errkind.NotFound {
		return nil, nil
	}
	return data, err
}

// project is a migrated project as compose names it.
func project(p hostplan.MigrateProject) host.Project {
	return host.Project{Name: p.Name, Dir: p.Dir, Files: p.Files}
}

// paths lists the paths written.
func paths(writes []hostplan.Write) []string {
	out := make([]string, 0, len(writes))
	for _, w := range writes {
		out = append(out, w.Path)
	}
	return out
}
