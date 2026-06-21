/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	healthhttp "github.com/kula-app/go-health/adapters/http"
	"github.com/kula-app/go-health/core"
)

func readyzCode(t *testing.T, eng *core.Engine) int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	healthhttp.ReadyzHandler(eng).ServeHTTP(rec, req)
	return rec.Code
}

// Feature: relay health
// Scenario: readiness gates on the SMTP listener being bound
//
//	Given a relay with a bound listener, then a closed one
//	When  readiness is evaluated
//	Then  it passes while bound and fails once closed
func TestHealthReadinessGatesOnListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	eng := NewHealthEngine(":"+portStr, nil, nil)

	if code := readyzCode(t, eng); code != http.StatusOK {
		t.Errorf("readyz with listener = %d, want 200", code)
	}

	_ = listener.Close()
	if code := readyzCode(t, eng); code == http.StatusOK {
		t.Errorf("readyz without listener = %d, want non-200", code)
	}
}

// Feature: relay health
// Scenario: destination reachability is healthz-only, not a readiness gate
//
//	Given an unreachable destination but a bound listener
//	When  readiness is evaluated
//	Then  readiness still passes
func TestHealthDestinationNotAReadinessGate(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	defer listener.Close()
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())

	unreachable := Target{Name: "down", HTTP: &HTTPTarget{URL: "http://127.0.0.1:1/down", Client: http.DefaultClient}}
	eng := NewHealthEngine(":"+portStr, []Target{unreachable}, nil)

	if code := readyzCode(t, eng); code != http.StatusOK {
		t.Errorf("readyz = %d, want 200 (destination must not gate readiness)", code)
	}
}

// Feature: relay health
// Scenario: a reachable destination is reported healthy
func TestHealthDestinationReachable(t *testing.T) {
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()

	target := Target{Name: "up", HTTP: &HTTPTarget{URL: dest.URL, Client: dest.Client()}}
	results := destinationReachable(context.Background(), &target)
	if len(results) != 1 || results[0].Status != core.StatusPass {
		t.Errorf("destination result = %+v, want pass", results)
	}
}

// Feature: relay health
// Scenario: an unreachable HTTP destination is reported failing
func TestHealthDestinationHTTPUnreachable(t *testing.T) {
	target := Target{Name: "down", HTTP: &HTTPTarget{URL: "http://127.0.0.1:1/down", Client: http.DefaultClient}}
	results := destinationReachable(context.Background(), &target)
	if len(results) != 1 || results[0].Status != core.StatusFail {
		t.Errorf("destination result = %+v, want fail", results)
	}
}

// Feature: relay health
// Scenario: a reachable SMTP destination is reported healthy
func TestHealthDestinationSMTPReachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	host, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	target := Target{Name: "smtp-up", SMTP: &SMTPTarget{Host: host, Port: port}}
	results := destinationReachable(context.Background(), &target)
	if len(results) != 1 || results[0].Status != core.StatusPass {
		t.Errorf("destination result = %+v, want pass", results)
	}
}

// Feature: relay health
// Scenario: an unreachable SMTP destination is reported failing
func TestHealthDestinationSMTPUnreachable(t *testing.T) {
	target := Target{Name: "smtp-down", SMTP: &SMTPTarget{Host: "127.0.0.1", Port: 1}}
	results := destinationReachable(context.Background(), &target)
	if len(results) != 1 || results[0].Status != core.StatusFail {
		t.Errorf("destination result = %+v, want fail", results)
	}
}

// Feature: relay health
// Scenario: a target with no delivery method is reported failing
//
//	Given a target with neither an HTTP nor an SMTP destination
//	When  reachability is probed
//	Then  it fails with a descriptive output
func TestHealthDestinationNoMethod(t *testing.T) {
	target := Target{Name: "empty"}
	results := destinationReachable(context.Background(), &target)
	if len(results) != 1 || results[0].Status != core.StatusFail {
		t.Fatalf("destination result = %+v, want fail", results)
	}
	if results[0].Output != "destination has no delivery method" {
		t.Errorf("output = %q, want the no-delivery-method message", results[0].Output)
	}
}
