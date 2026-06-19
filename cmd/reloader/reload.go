/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// commandRunner runs an external command. It is a seam so the reload logic can
// be tested without invoking postfix.
type commandRunner func(ctx context.Context, name string, args ...string) error

// execRunner runs a command and returns its combined output on failure.
func execRunner(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return nil
}

// reload runs a Postfix reload and records the iris_postfix_* metrics: the
// attempt result, its latency, and the timestamp of the last success.
func reload(ctx context.Context, run commandRunner) error {
	start := time.Now()
	err := reloadPostfix(ctx, run)
	reloadDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		reloadsTotal.WithLabelValues("failure").Inc()
		return err
	}
	reloadsTotal.WithLabelValues("success").Inc()
	lastReloadTimestamp.SetToCurrentTime()
	return nil
}

// reloadPostfix reloads Postfix so its daemons re-read the routing maps. The
// maps are mounted as texthash files that Postfix reads directly, so no postmap
// compilation step is needed (the ConfigMap mount is read-only anyway).
func reloadPostfix(ctx context.Context, run commandRunner) error {
	if err := run(ctx, "postfix", "reload"); err != nil {
		return fmt.Errorf("postfix reload: %w", err)
	}
	return nil
}
