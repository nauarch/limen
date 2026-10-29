// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/kernel/bytesize"
)

// errRefused is a command whose plan was refused; the plan printed why.
var errRefused = errors.New("refused: nothing was changed")

// migrate moves the host to the containerd image store: it prints every step, and takes them only
// with --yes.
func migrate(ctx context.Context, args []string, o Options) error {
	set := flags("migrate", o)
	limen := set.String("limen", "", "the limen to point the host at and fill ahead, by IP, such as http://10.0.0.5:5000")
	prune := set.Bool("prune", false, "remove images no container uses first")
	force := set.Bool("force", false, "go ahead despite warnings (containers not from compose, DNS served only by the host)")
	yes := set.Bool("yes", false, "make the move (without it, only print it)")
	if err := parse(set, args); err != nil {
		return err
	}
	m, err := o.Migrator.PlanMigration(ctx, hostplan.MigrateRequest{Limen: *limen, Prune: *prune, Force: *force})
	if err != nil {
		return err
	}
	return carry(ctx, o, m, *yes, o.Migrator.Migrate)
}

// rollback moves a migrated host back to the classic image store.
func rollback(ctx context.Context, args []string, o Options) error {
	set := flags("rollback", o)
	yes := set.Bool("yes", false, "make the move (without it, only print it)")
	if err := parse(set, args); err != nil {
		return err
	}
	m, err := o.Migrator.PlanRollback(ctx)
	if err != nil {
		return err
	}
	return carry(ctx, o, m, *yes, o.Migrator.Rollback)
}

// cleanup removes the classic store's data.
func cleanup(ctx context.Context, args []string, o Options) error {
	set := flags("cleanup-old-store", o)
	force := set.Bool("force", false, "clean up without a finished migration on record (a host moved by hand)")
	yes := set.Bool("yes", false, "remove the data (without it, only print what would go)")
	if err := parse(set, args); err != nil {
		return err
	}
	c, err := o.Migrator.PlanCleanup(ctx, *force)
	if err != nil {
		return err
	}
	if refused(o.Out, c.Refusals) {
		return errRefused
	}
	var total int64
	fmt.Fprintln(o.Out, "The classic image store's data to remove:")
	for _, p := range c.Paths {
		fmt.Fprintf(o.Out, "  %s  %s\n", p.Path, bytesize.Format(p.Bytes))
		total += p.Bytes
	}
	fmt.Fprintf(o.Out, "  %s in all. After this the host cannot roll back.\n", bytesize.Format(total))
	return finish(o, *yes, func(progress func(string)) error { return o.Migrator.Cleanup(ctx, c, progress) })
}

// carry prints a move between stores and, with yes, makes it.
func carry(ctx context.Context, o Options, m hostplan.Migration, yes bool, move func(context.Context, hostplan.Migration, func(string)) error) error {
	if refused(o.Out, m.Refusals) {
		printNotes(o.Out, m.Notes)
		return errRefused
	}
	printMigration(o.Out, m)
	return finish(o, yes, func(progress func(string)) error { return move(ctx, m, progress) })
}

// finish says nothing changed without yes; with it, does the work, printing each step.
func finish(o Options, yes bool, do func(progress func(string)) error) error {
	if !yes {
		fmt.Fprintln(o.Out, "\nDry run: nothing was changed. Run again with --yes to go ahead.")
		return nil
	}
	fmt.Fprintln(o.Out)
	start := time.Now()
	if err := do(func(step string) { fmt.Fprintf(o.Out, "-> %s\n", step) }); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "\nDone in %s.\n", time.Since(start).Round(time.Second))
	return nil
}

// refused prints why a plan was refused, reporting whether it was.
func refused(w io.Writer, why []string) bool {
	if len(why) == 0 {
		return false
	}
	fmt.Fprintln(w, "Refused:")
	for _, r := range why {
		fmt.Fprintf(w, "  - %s\n", r)
	}
	return true
}

// printMigration writes a move's steps in the order they are taken.
func printMigration(w io.Writer, m hostplan.Migration) {
	fmt.Fprintf(w, "Moving to the %s image store:\n", m.To)
	step := 0
	say := func(format string, a ...any) {
		step++
		fmt.Fprintf(w, "\n%d. %s\n", step, fmt.Sprintf(format, a...))
	}
	if len(m.Keep) > 0 {
		say("save the images built on this host to %s (about %s; loaded back after the switch)", m.KeepPath, bytesize.Format(m.KeepBytes))
		for _, ref := range m.Keep {
			fmt.Fprintf(w, "   %s\n", ref)
		}
	}
	if m.Prune {
		say("remove images no container uses (up to %s)", bytesize.Format(m.PruneBytes))
	}
	if m.Limen != "" && len(m.Prefetch) > 0 {
		say("limen %s fetches %d images for %s ahead (nothing stops until it has)", m.Limen, len(m.Prefetch), m.Platform)
		for _, ref := range m.Prefetch {
			fmt.Fprintf(w, "   %s\n", ref)
		}
	}
	say("stop the projects")
	for _, p := range slices.Backward(m.Projects) {
		fmt.Fprintf(w, "   %s: %s\n", p.Name, how(p.Stop, "docker compose stop"))
	}
	say("write the files")
	printChanges(w, hostplan.Changes{Writes: m.Writes, Limen: m.Limen})
	say("restart Docker on the %s image store", m.To)
	if len(m.Keep) > 0 {
		say("load the saved images into the new store")
	}
	say("start the projects")
	for _, p := range m.Projects {
		fmt.Fprintf(w, "   %s: %s\n", p.Name, how(p.Start, "docker compose up --detach"))
	}
	printNotes(w, m.Notes)
}

// how names a hook, or compose's own command when there is none.
func how(hook, compose string) string {
	if hook != "" {
		return hook
	}
	return compose
}

// printNotes writes what is worth knowing.
func printNotes(w io.Writer, notes []string) {
	if len(notes) == 0 {
		return
	}
	fmt.Fprintln(w, "\nWorth knowing:")
	for _, n := range notes {
		fmt.Fprintf(w, "  - %s\n", n)
	}
}
