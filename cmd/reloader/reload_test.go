/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Feature: postfix reload
// Scenario: source maps are copied into the writable dir, compiled, then loaded
//
//	Given a read-only source dir with transport and relay_recipient_maps
//	When  a reload runs
//	Then  the maps are copied to the work dir, postmap runs for each hashed map,
//	      and postfix reload runs last
func TestReloadCopiesCompilesThenReloads(t *testing.T) {
	src := t.TempDir()
	work := t.TempDir()
	mustWrite(t, filepath.Join(src, "transport"), "invite.example.com smtp:[relay]:25\n")
	mustWrite(t, filepath.Join(src, "relay_recipient_maps"), "@invite.example.com OK\n")
	mustWrite(t, filepath.Join(src, "relay_domains"), "invite.example.com\n")

	var calls []string
	runner := func(_ context.Context, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := reloadPostfix(context.Background(), src, work, runner); err != nil {
		t.Fatalf("reload: %v", err)
	}

	// The maps are copied into the writable work dir.
	for _, name := range []string{"transport", "relay_recipient_maps", "relay_domains"} {
		if _, err := os.Stat(filepath.Join(work, name)); err != nil {
			t.Errorf("map %s not copied to work dir: %v", name, err)
		}
	}

	if len(calls) != 3 {
		t.Fatalf("calls = %v, want 3", calls)
	}
	if !strings.HasPrefix(calls[0], "postmap "+work) || !strings.HasPrefix(calls[1], "postmap "+work) {
		t.Errorf("expected two postmap calls against the work dir first, got %v", calls)
	}
	if calls[2] != "postfix reload" {
		t.Errorf("last call = %q, want postfix reload", calls[2])
	}
}

// Feature: postfix reload
// Scenario: a postmap failure aborts before the reload
//
//	Given postmap fails
//	When  a reload runs
//	Then  postfix reload is not attempted, so the ingress keeps the old maps
func TestReloadAbortsOnPostmapFailure(t *testing.T) {
	src := t.TempDir()
	work := t.TempDir()
	mustWrite(t, filepath.Join(src, "transport"), "x")

	var reloadCalled bool
	runner := func(_ context.Context, name string, _ ...string) error {
		if name == "postmap" {
			return errors.New("boom")
		}
		if name == "postfix" {
			reloadCalled = true
		}
		return nil
	}

	if err := reloadPostfix(context.Background(), src, work, runner); err == nil {
		t.Error("expected error when postmap fails")
	}
	if reloadCalled {
		t.Error("postfix reload should not run after a postmap failure")
	}
}

// Feature: postfix reload
// Scenario: reload outcomes are metered
//
//	Given a reload that succeeds and one that fails
//	When  each runs
//	Then  the matching iris_postfix_reloads_total series increments
func TestReloadRecordsMetrics(t *testing.T) {
	reloadsTotal.Reset()
	src := t.TempDir()
	work := t.TempDir()
	mustWrite(t, filepath.Join(src, "transport"), "x")

	okRunner := func(_ context.Context, _ string, _ ...string) error { return nil }
	if err := reload(context.Background(), src, work, okRunner); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := testutil.ToFloat64(reloadsTotal.WithLabelValues("success")); got != 1 {
		t.Errorf("success reloads = %v, want 1", got)
	}

	failRunner := func(_ context.Context, _ string, _ ...string) error { return errors.New("boom") }
	if err := reload(context.Background(), src, work, failRunner); err == nil {
		t.Error("expected reload error")
	}
	if got := testutil.ToFloat64(reloadsTotal.WithLabelValues("failure")); got != 1 {
		t.Errorf("failure reloads = %v, want 1", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
