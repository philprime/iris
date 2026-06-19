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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Feature: postfix reload
// Scenario: a map change triggers a postfix reload
//
//	Given the maps are mounted as texthash files Postfix reads directly
//	When  a reload runs
//	Then  postfix reload runs so the daemons re-read the maps, with no postmap
func TestReloadPostfixReloads(t *testing.T) {
	var calls []string
	runner := func(_ context.Context, name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	if err := reloadPostfix(context.Background(), runner); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if len(calls) != 1 || calls[0] != "postfix reload" {
		t.Fatalf("calls = %v, want [postfix reload]", calls)
	}
}

// Feature: postfix reload
// Scenario: a reload failure is surfaced
//
//	Given postfix reload fails
//	When  a reload runs
//	Then  the error is returned so the failure is metered and logged
func TestReloadPostfixReturnsError(t *testing.T) {
	runner := func(_ context.Context, _ string, _ ...string) error {
		return errors.New("boom")
	}

	if err := reloadPostfix(context.Background(), runner); err == nil {
		t.Error("expected error when postfix reload fails")
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

	okRunner := func(_ context.Context, _ string, _ ...string) error { return nil }
	if err := reload(context.Background(), okRunner); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := testutil.ToFloat64(reloadsTotal.WithLabelValues("success")); got != 1 {
		t.Errorf("success reloads = %v, want 1", got)
	}

	failRunner := func(_ context.Context, _ string, _ ...string) error { return errors.New("boom") }
	if err := reload(context.Background(), failRunner); err == nil {
		t.Error("expected reload error")
	}
	if got := testutil.ToFloat64(reloadsTotal.WithLabelValues("failure")); got != 1 {
		t.Errorf("failure reloads = %v, want 1", got)
	}
}
