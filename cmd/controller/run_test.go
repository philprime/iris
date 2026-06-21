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

// Feature: webhook bind address parsing
// Scenario: a "host:port" address is split into host and integer port
//
//	Given various bind addresses
//	When  splitHostPort parses them
//	Then  valid addresses yield the host and port and malformed ones error
func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{name: "all interfaces", addr: ":9443", wantHost: "", wantPort: 9443},
		{name: "explicit host", addr: "0.0.0.0:8443", wantHost: "0.0.0.0", wantPort: 8443},
		{name: "loopback", addr: "127.0.0.1:443", wantHost: "127.0.0.1", wantPort: 443},
		{name: "missing port", addr: "bad", wantErr: true},
		{name: "non-numeric port", addr: "host:nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := splitHostPort(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitHostPort(%q) = nil error, want error", tt.addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitHostPort(%q): %v", tt.addr, err)
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Errorf("splitHostPort(%q) = (%q, %d), want (%q, %d)", tt.addr, host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}

// Feature: Sentry release resolution
// Scenario: the ldflags-injected release wins, otherwise it is derived
//
//	Given the sentryRelease build var set or empty
//	When  sentryReleaseID resolves the release
//	Then  a set value is returned verbatim and an empty one falls back to
//	      iris@<version>:<commit>
func TestSentryReleaseID(t *testing.T) {
	original := sentryRelease
	t.Cleanup(func() { sentryRelease = original })

	sentryRelease = ""
	if got, want := sentryReleaseID(), "iris@dev:none"; got != want {
		t.Errorf("sentryReleaseID() = %q, want %q", got, want)
	}

	sentryRelease = "iris@1.2.3:abc123"
	if got, want := sentryReleaseID(), "iris@1.2.3:abc123"; got != want {
		t.Errorf("sentryReleaseID() = %q, want %q", got, want)
	}
}

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
