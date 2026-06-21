/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// Feature: reloader startup validation
// Scenario: an invalid configuration aborts before watching begins
func TestRunReturnsConfigError(t *testing.T) {
	t.Setenv("IRIS_SENTRY_SAMPLE_RATE", "2.0")
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("run() error = %v, want load config error", err)
	}
}

// Feature: reloader startup validation
// Scenario: an unwatchable source path aborts startup
//
//	Given a source path that does not exist
//	When  run sets up the file watch
//	Then  it returns a watch error
func TestRunReturnsWatchError(t *testing.T) {
	t.Setenv("IRIS_RELOADER_ADMIN_ADDR", "127.0.0.1:0")
	t.Setenv("IRIS_RELOADER_SOURCE_PATH", filepath.Join(t.TempDir(), "missing"))
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "watch") {
		t.Fatalf("run() error = %v, want watch error", err)
	}
}
