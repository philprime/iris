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

	sentryRelease = "iris@7.7.7:cafe"
	if got, want := sentryReleaseID(), "iris@7.7.7:cafe"; got != want {
		t.Errorf("sentryReleaseID() = %q, want %q", got, want)
	}
}

// Feature: reloader admin server
// Scenario: the admin mux exposes the health and metrics endpoints
//
//	Given an admin mux built around a health engine
//	When  each admin path is requested
//	Then  a handler is registered for it (no 404)
func TestAdminMux(t *testing.T) {
	eng := newHealthEngine(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	mux := adminMux(eng)
	for _, path := range []string{"/livez", "/readyz", "/metrics"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusNotFound {
			t.Errorf("admin path %s returned 404, want a registered handler", path)
		}
	}
}

// Feature: reloader admin server
// Scenario: shutdownAdmin gracefully stops a running admin server
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
