// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
)

// errAround is a pull that did not reach limen; verify has said so.
var errAround = errors.New("the pull went around limen")

// verify pulls a small image and checks that the pull reached limen.
func verify(ctx context.Context, args []string, o Options) error {
	set := flags("verify", o)
	limen := set.String("limen", "", "the limen the host should pull through, such as http://10.0.0.5:5000 (required)")
	if err := parse(set, args); err != nil {
		return err
	}
	if *limen == "" {
		fmt.Fprintln(o.Err, "verify needs --limen")
		set.Usage()
		return errUsage
	}
	v, err := o.Planner.Verify(ctx, *limen)
	if err != nil {
		return err
	}
	if v.Removed {
		fmt.Fprintf(o.Out, "Pulled %s, and removed it again.\n", v.Image)
	} else {
		fmt.Fprintf(o.Out, "Pulled %s (the host had it already).\n", v.Image)
	}
	if v.Requests <= 0 {
		fmt.Fprintf(o.Out, "limen at %s counted no requests from this host: the pull went around it.\nRun limenctl plan --limen %s to see why.\n", *limen, *limen)
		return errAround
	}
	fmt.Fprintf(o.Out, "limen at %s counted %d requests from this host during the pull: pulls go through limen.\n", *limen, v.Requests)
	return nil
}
