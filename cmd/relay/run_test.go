/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/philprime/iris/internal/relay"
)

// Feature: Sentry release resolution
// Scenario: the ldflags-injected release wins, otherwise it is derived
func TestSentryReleaseID(t *testing.T) {
	original := sentryRelease
	t.Cleanup(func() { sentryRelease = original })

	sentryRelease = ""
	if got, want := sentryReleaseID(), "iris@dev:none"; got != want {
		t.Errorf("sentryReleaseID() = %q, want %q", got, want)
	}

	sentryRelease = "iris@9.9.9:deadbee"
	if got, want := sentryReleaseID(), "iris@9.9.9:deadbee"; got != want {
		t.Errorf("sentryReleaseID() = %q, want %q", got, want)
	}
}

// Feature: relay admin server
// Scenario: the admin mux exposes the health and metrics endpoints
//
//	Given an admin mux built around a health engine
//	When  each admin path is requested
//	Then  a handler is registered for it (no 404)
func TestAdminMux(t *testing.T) {
	eng := relay.NewHealthEngine(":0", nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	mux := adminMux(eng)
	for _, path := range []string{"/livez", "/readyz", "/healthz", "/metrics"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusNotFound {
			t.Errorf("admin path %s returned 404, want a registered handler", path)
		}
	}
}

// Feature: relay admin server
// Scenario: shutdownAdmin gracefully stops a running admin server
//
//	Given a serving admin server
//	When  shutdownAdmin is called
//	Then  the server stops accepting requests
func TestShutdownAdmin(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: http.NewServeMux(), ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()

	shutdownAdmin(srv)

	if _, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
		t.Error("server still accepting requests after shutdownAdmin")
	}
}

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
