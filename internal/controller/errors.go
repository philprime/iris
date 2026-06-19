/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package controller

import "errors"

// terminalError marks a reconcile failure as permanent (a config or spec
// problem) rather than transient. Terminal errors set Ready=False, are reported
// to Sentry, and do not requeue, since retrying cannot fix them. Transient
// errors are returned plain so the controller requeues with backoff.
type terminalError struct {
	err error
}

func (e *terminalError) Error() string { return e.err.Error() }
func (e *terminalError) Unwrap() error { return e.err }

// terminal wraps an error as terminal.
func terminal(err error) error {
	if err == nil {
		return nil
	}
	return &terminalError{err: err}
}

// isTerminal reports whether err (or anything it wraps) is terminal.
func isTerminal(err error) bool {
	var t *terminalError
	return errors.As(err, &t)
}
