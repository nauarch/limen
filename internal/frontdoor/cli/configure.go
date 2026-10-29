// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/nauarch/limen/internal/contracts/hostplan"
)

// configure points the host at a limen: it prints every change, and makes them only with --yes.
func configure(ctx context.Context, args []string, o Options) error {
	set := flags("configure", o)
	limen := set.String("limen", "", "the limen to point the host at, by IP so pulls need no DNS, such as http://10.0.0.5:5000 (required)")
	force := set.Bool("force", false, "replace hosts.toml files limenctl did not write")
	yes := set.Bool("yes", false, "make the changes (without it, only print them)")
	if err := parse(set, args); err != nil {
		return err
	}
	if *limen == "" {
		fmt.Fprintln(o.Err, "configure needs --limen")
		set.Usage()
		return errUsage
	}
	ch, err := o.Planner.Configure(ctx, hostplan.ConfigureRequest{Limen: *limen, Force: *force})
	if err != nil {
		return err
	}
	printChanges(o.Out, ch)
	if len(ch.Writes) == 0 {
		return nil
	}
	if !*yes {
		fmt.Fprintln(o.Out, "\nDry run: nothing was changed. Run again with --yes to make these changes.")
		return nil
	}
	start := time.Now()
	if err := o.Planner.Apply(ctx, ch); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "\nDone in %s. To check that pulls reach limen: limenctl verify --limen %s\n", time.Since(start).Round(time.Second), *limen)
	return nil
}

// printChanges writes each file change in full, then what was left alone. New hosts.toml files,
// alike but for their registry, are listed together with one shown in full.
func printChanges(w io.Writer, ch hostplan.Changes) {
	if len(ch.Writes) == 0 {
		fmt.Fprintf(w, "The host already points at limen %s: nothing to change.\n", ch.Limen)
	}
	fresh := func(c hostplan.Write) bool { return c.Before == nil && strings.HasSuffix(c.Path, "/hosts.toml") }
	var registries []string
	for _, c := range ch.Writes {
		if fresh(c) {
			registries = append(registries, path.Base(path.Dir(c.Path)))
		}
	}
	grouped := len(registries) > 1
	if grouped {
		first := ch.Writes[slices.IndexFunc(ch.Writes, fresh)]
		fmt.Fprintf(w, "\nhosts.toml for %d registries (new): %s\n", len(registries), strings.Join(registries, ", "))
		fmt.Fprintf(w, "each like %s:\n", first.Path)
		fmt.Fprint(w, indent("+ ", first.After))
	}
	for _, c := range ch.Writes {
		if grouped && fresh(c) {
			continue
		}
		state := "new"
		switch {
		case c.After == nil:
			state = "removed, a backup is kept"
		case c.Before != nil:
			state = "changed, the old one is backed up"
		}
		fmt.Fprintf(w, "\n%s (%s)\n", c.Path, state)
		if c.Before != nil {
			fmt.Fprint(w, indent("- ", c.Before))
		}
		fmt.Fprint(w, indent("+ ", c.After))
	}
	for _, s := range ch.Skipped {
		fmt.Fprintf(w, "\nleft alone: %s\n", s)
	}
	if ch.Restart {
		fmt.Fprintln(w, "\nDocker restarts to read the changes: its running containers restart with it.")
	}
}

// indent prefixes each line of data.
func indent(prefix string, data []byte) string {
	var b strings.Builder
	for line := range strings.Lines(string(data)) {
		b.WriteString(prefix + strings.TrimRight(line, "\n") + "\n")
	}
	return b.String()
}

// unconfigure takes limenctl's configuration off the host: it prints every change, and makes them
// only with --yes.
func unconfigure(ctx context.Context, args []string, o Options) error {
	set := flags("unconfigure", o)
	limen := set.String("limen", "", "a limen to take out of daemon.json's mirrors too (the ones limenctl's hosts.toml files name always go)")
	yes := set.Bool("yes", false, "make the changes (without it, only print them)")
	if err := parse(set, args); err != nil {
		return err
	}
	ch, err := o.Planner.Unconfigure(ctx, *limen)
	if err != nil {
		return err
	}
	if len(ch.Writes) == 0 {
		fmt.Fprintln(o.Out, "limenctl has configured nothing here: nothing to undo.")
		printChanges(o.Out, ch)
		return nil
	}
	printChanges(o.Out, ch)
	return finish(o, *yes, func(func(string)) error { return o.Planner.Apply(ctx, ch) })
}
