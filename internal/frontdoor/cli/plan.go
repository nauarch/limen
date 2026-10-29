// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/kernel/bytesize"
)

// plan reports on the host.
func plan(ctx context.Context, args []string, o Options) error {
	set := flags("plan", o)
	limen := set.String("limen", "", "the limen to check the host against, such as http://10.0.0.5:5000")
	if err := parse(set, args); err != nil {
		return err
	}
	r, err := o.Planner.Report(ctx, *limen)
	if err != nil {
		return err
	}
	printReport(o.Out, r)
	return nil
}

// printReport writes the report as sections.
func printReport(w io.Writer, r hostplan.Report) {
	fmt.Fprintf(w, "Docker %s, %s image store\n", r.Version, r.Store)

	fmt.Fprintln(w, "\nCompose projects")
	t := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(r.Projects) == 0 {
		fmt.Fprintln(t, "  (none)")
	}
	for _, p := range r.Projects {
		line := fmt.Sprintf("  %s\t%s\t%d/%d running", p.Name, p.Dir, p.Running, p.Total)
		if len(p.Scripts) > 0 {
			line += "\tscripts: " + strings.Join(p.Scripts, ", ")
		}
		fmt.Fprintln(t, line)
	}
	_ = t.Flush()
	if len(r.Loose) > 0 {
		fmt.Fprintf(w, "  not from compose: %s\n", strings.Join(r.Loose, ", "))
	}

	fmt.Fprintln(w, "\nImages (sizes summed per image: layers they share count more than once)")
	t = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(t, "  in use\t%d\t%s\n", r.InUse.Count, bytesize.Format(r.InUse.Bytes))
	fmt.Fprintf(t, "  unused\t%d\t%s\n", r.Unused.Count, bytesize.Format(r.Unused.Bytes))
	_ = t.Flush()

	fmt.Fprintln(w, "\nDisk")
	t = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, d := range r.Disk {
		free := "free space unknown"
		if d.Free >= 0 {
			free = bytesize.Format(d.Free) + " free"
		}
		fmt.Fprintf(t, "  %s\t%s\n", d.Path, free)
	}
	_ = t.Flush()

	if m := r.Mirror; m.Limen != "" {
		fmt.Fprintf(w, "\nlimen %s\n", m.Limen)
		switch {
		case m.Unreachable != "":
			fmt.Fprintf(w, "  unreachable: %s\n", m.Unreachable)
		default:
			fmt.Fprintf(w, "  serves:      %s\n", strings.Join(m.Registries, ", "))
			fmt.Fprintf(w, "  hosts.toml:  %s\n", orNone(m.Configured))
		}
		fmt.Fprintf(w, "  daemon.json: registry-mirrors %s\n", orNone(m.Daemon))
	}

	fmt.Fprintln(w, "\nFindings")
	for _, f := range r.Findings {
		fmt.Fprintf(w, "  %-5s %s\n", f.Level, f.Text)
	}
}

// orNone joins a list, or says none.
func orNone(list []string) string {
	if len(list) == 0 {
		return "(none)"
	}
	return strings.Join(list, ", ")
}
