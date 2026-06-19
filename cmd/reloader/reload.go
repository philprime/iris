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
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// hashedMaps are the Postfix map files compiled with postmap. relay_domains is
// a plain list and is only copied, not hashed.
var hashedMaps = []string{"transport", "relay_recipient_maps"}

// copiedMaps are every map file synced from the read-only source mount into the
// writable work directory Postfix reads.
var copiedMaps = []string{"transport", "relay_recipient_maps", "relay_domains"}

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
func reload(ctx context.Context, sourceDir, workDir string, run commandRunner) error {
	start := time.Now()
	err := reloadPostfix(ctx, sourceDir, workDir, run)
	reloadDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		reloadsTotal.WithLabelValues("failure").Inc()
		return err
	}
	reloadsTotal.WithLabelValues("success").Inc()
	lastReloadTimestamp.SetToCurrentTime()
	return nil
}

// reloadPostfix syncs the maps from the read-only source mount into the writable
// work directory, compiles each hashed map with postmap, then reloads Postfix.
// The copy is needed because postmap writes its compiled database next to the
// source file, which the read-only ConfigMap mount forbids. A postmap failure
// aborts before the reload so the ingress is not reloaded against a half-built
// map.
func reloadPostfix(ctx context.Context, sourceDir, workDir string, run commandRunner) error {
	if err := syncMaps(sourceDir, workDir); err != nil {
		return fmt.Errorf("sync maps: %w", err)
	}
	for _, name := range hashedMaps {
		path := filepath.Join(workDir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := run(ctx, "postmap", path); err != nil {
			return fmt.Errorf("postmap %s: %w", name, err)
		}
	}
	return run(ctx, "postfix", "reload")
}

// syncMaps copies each present map from the source mount into the work
// directory, so the compiled databases land on a writable filesystem.
func syncMaps(sourceDir, workDir string) error {
	for _, name := range copiedMaps {
		data, err := os.ReadFile(filepath.Join(sourceDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(workDir, name), data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
