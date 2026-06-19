/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/philprime/iris/api/v1alpha1"
)

func httpTarget(t *testing.T, status int) (*HTTPTarget, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	return &HTTPTarget{URL: srv.URL, Format: v1alpha1.PayloadFormatJSON, Client: srv.Client()}, srv.Close
}

// Feature: fan-out delivery contract
// Scenario: a best-effort failure does not fail the batch
//
//	Given a required destination that succeeds and a best-effort one that fails
//	When  the message is fanned out
//	Then  no required destination failed, so the session returns 250
func TestFanOutBestEffortFailureDoesNotGate(t *testing.T) {
	ok, closeOK := httpTarget(t, http.StatusOK)
	defer closeOK()
	bad, closeBad := httpTarget(t, http.StatusInternalServerError)
	defer closeBad()

	targets := []Target{
		{Name: "req", Required: true, HTTP: ok},
		{Name: "opt", Required: false, HTTP: bad},
	}
	res := FanOut(context.Background(), targets, sampleEnvelope(), []byte("RAW"))

	if res.RequiredFailed {
		t.Error("best-effort failure should not mark the batch as required-failed")
	}
	if res.Results["opt"] == nil {
		t.Error("expected the best-effort failure to be recorded")
	}
}

// Feature: fan-out delivery contract
// Scenario: a required failure gates the batch
//
//	Given a required destination that fails
//	When  the message is fanned out
//	Then  a required destination failed, so the session returns 4xx for retry
func TestFanOutRequiredFailureGates(t *testing.T) {
	bad, closeBad := httpTarget(t, http.StatusInternalServerError)
	defer closeBad()

	targets := []Target{{Name: "req", Required: true, HTTP: bad}}
	res := FanOut(context.Background(), targets, sampleEnvelope(), []byte("RAW"))

	if !res.RequiredFailed {
		t.Error("required failure should mark the batch as required-failed")
	}
}
