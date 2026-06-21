/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package adminserver

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kula-app/go-health/core"
)

// Feature: admin server
// Scenario: the server exposes the health and metrics endpoints
//
//	Given an admin server built around a health engine
//	When  each admin path is requested
//	Then  a handler is registered for it (no 404)
func TestNewServesEndpoints(t *testing.T) {
	eng := core.NewEngine("test", "admin server test")
	srv := New(":0", eng)
	for _, path := range []string{"/livez", "/readyz", "/healthz", "/metrics"} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code == http.StatusNotFound {
			t.Errorf("admin path %s returned 404, want a registered handler", path)
		}
	}
}

// Feature: admin server
// Scenario: Shutdown gracefully stops a running admin server
//
//	Given a serving admin server
//	When  Shutdown is called
//	Then  the server stops accepting requests
func TestShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: http.NewServeMux(), ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()

	Shutdown(srv)

	if _, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
		t.Error("server still accepting requests after Shutdown")
	}
}
