// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/nauarch/limen/internal/contracts/hostplan"
	"github.com/nauarch/limen/internal/kernel/build"
)

// usage is the help printed for no command or an unknown one.
const usage = `limenctl points a Docker host at limen, a pull-through cache for container registries.

Commands:
  plan       read the host and report what limenctl sees and would change (changes nothing)
  configure  point the host at a limen: a dry run that only writes with --yes
  unconfigure
             take limenctl's configuration off the host (its hosts.toml files, limen in daemon.json)
  verify     pull a small image and check the pull reached limen
  migrate    move the host to the containerd image store, its projects stopped and started again
  rollback   move a migrated host back to the classic image store
  cleanup-old-store
             remove the classic store's data once the host has moved for good (no way back)
  version    print limenctl's version

Every command that changes the host only prints what it would do, unless given --yes.

Run limenctl <command> -h for a command's flags.
`

// Options are what the command line drives, the version it reports, and where it prints.
type Options struct {
	Planner  hostplan.Planner
	Migrator hostplan.Migrator
	Version  string
	Out      io.Writer
	Err      io.Writer
}

// Run runs one command and gives its exit code: 0 done, 1 failed, 2 misused.
func Run(ctx context.Context, args []string, o Options) int {
	if len(args) == 0 {
		fmt.Fprint(o.Err, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "plan":
		err = plan(ctx, args[1:], o)
	case "configure":
		err = configure(ctx, args[1:], o)
	case "unconfigure":
		err = unconfigure(ctx, args[1:], o)
	case "verify":
		err = verify(ctx, args[1:], o)
	case "migrate":
		err = migrate(ctx, args[1:], o)
	case "rollback":
		err = rollback(ctx, args[1:], o)
	case "cleanup-old-store":
		err = cleanup(ctx, args[1:], o)
	case "version", "--version":
		fmt.Fprintln(o.Out, build.About("limenctl", o.Version))
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(o.Out, usage)
		return 0
	default:
		fmt.Fprintf(o.Err, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp), errors.Is(err, errUsage):
		return 2
	default:
		fmt.Fprintln(o.Err, "limenctl:", err)
		return 1
	}
}

// errUsage is a command line that cannot run; its flags have said why.
var errUsage = errors.New("usage")

// flags is a command's flag set, printing to o.Err.
func flags(name string, o Options) *flag.FlagSet {
	set := flag.NewFlagSet("limenctl "+name, flag.ContinueOnError)
	set.SetOutput(o.Err)
	return set
}

// parse reads a command's flags; flags it cannot read are misuse, and the set has said why.
func parse(set *flag.FlagSet, args []string) error {
	if err := set.Parse(args); err != nil {
		return errUsage
	}
	return nil
}
