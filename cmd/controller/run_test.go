/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"strings"
	"testing"
)

// Feature: controller startup validation
// Scenario: an invalid configuration aborts before the manager starts
//
//	Given an out-of-range Sentry sample rate
//	When  run loads the configuration
//	Then  it returns a load config error
func TestRunReturnsConfigError(t *testing.T) {
	t.Setenv("IRIS_SENTRY_SAMPLE_RATE", "2.0")
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("run() error = %v, want load config error", err)
	}
}

// Feature: controller startup validation
// Scenario: a malformed webhook address aborts before the manager starts
//
//	Given a webhook bind address without a port
//	When  run parses the address
//	Then  it returns a parse webhook address error
func TestRunReturnsWebhookAddrError(t *testing.T) {
	t.Setenv("IRIS_CONTROLLER_WEBHOOK_ADDR", "missing-port")
	err := run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "parse webhook address") {
		t.Fatalf("run() error = %v, want parse webhook address error", err)
	}
}
