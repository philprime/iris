/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Feature: relay startup validation
// Scenario: an invalid configuration aborts before the servers start
func TestRunReturnsConfigError(t *testing.T) {
	t.Setenv("IRIS_SENTRY_SAMPLE_RATE", "2.0")
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("run() error = %v, want load config error", err)
	}
}

// Feature: relay startup validation
// Scenario: a missing relay config file aborts before the servers start
func TestRunReturnsReadConfigError(t *testing.T) {
	t.Setenv("IRIS_RELAY_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "read relay config") {
		t.Fatalf("run() error = %v, want read relay config error", err)
	}
}

// Feature: relay startup validation
// Scenario: a malformed relay config aborts before the servers start
func TestRunReturnsParseConfigError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("not: [valid"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("IRIS_RELAY_CONFIG", path)
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "parse relay config") {
		t.Fatalf("run() error = %v, want parse relay config error", err)
	}
}
