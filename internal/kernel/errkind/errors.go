// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package errkind

import "errors"

// The sentinels a failure wraps to say its kind.
var (
	// ErrNotFound: the thing asked for does not exist, here or upstream.
	ErrNotFound = errors.New("not found")
	// ErrInvalid: the request itself is malformed.
	ErrInvalid = errors.New("invalid")
	// ErrForbidden: the request is well formed but not served, such as a registry off the allow-list.
	ErrForbidden = errors.New("forbidden")
	// ErrConflict: the thing is held by someone else, such as a store another limen runs on.
	ErrConflict = errors.New("conflict")
	// ErrUpstream: the upstream registry answered, but wrongly (an error status, a bad digest).
	ErrUpstream = errors.New("upstream failed")
	// ErrUnreachable: the upstream registry could not be reached at all.
	ErrUnreachable = errors.New("unreachable")
)
