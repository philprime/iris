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
)

// hashedMaps are the Postfix map files that must be compiled with postmap.
// relay_domains is a plain list and is not hashed.
var hashedMaps = []string{"transport", "relay_recipient_maps"}

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

// reloadPostfix compiles each present hashed map with postmap, then reloads
// Postfix. A postmap failure aborts before the reload so the ingress is not
// reloaded against a half-built map.
func reloadPostfix(ctx context.Context, dir string, run commandRunner) error {
	for _, name := range hashedMaps {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := run(ctx, "postmap", path); err != nil {
			return fmt.Errorf("postmap %s: %w", name, err)
		}
	}
	return run(ctx, "postfix", "reload")
}
