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
)

// Feature: postfix reload
// Scenario: present maps are compiled before the reload
//
//	Given a maps directory with transport and relay_recipient_maps
//	When  a reload runs
//	Then  postmap runs for each map and postfix reload runs last
func TestReloadPostfixCompilesThenReloads(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "transport"), "x")
	mustWrite(t, filepath.Join(dir, "relay_recipient_maps"), "y")

	var calls []string
	runner := func(_ context.Context, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := reloadPostfix(context.Background(), dir, runner); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("calls = %v, want 3", calls)
	}
	if !strings.HasPrefix(calls[0], "postmap ") || !strings.HasPrefix(calls[1], "postmap ") {
		t.Errorf("expected two postmap calls first, got %v", calls)
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
func TestReloadPostfixAbortsOnPostmapFailure(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "transport"), "x")

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

	if err := reloadPostfix(context.Background(), dir, runner); err == nil {
		t.Error("expected error when postmap fails")
	}
	if reloadCalled {
		t.Error("postfix reload should not run after a postmap failure")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
