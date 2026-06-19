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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

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

// Feature: fan-out delivery contract
// Scenario: destinations are delivered to concurrently
//
//	Given two destinations that each block until both have been entered
//	When  the message is fanned out
//	Then  both handlers are active at once, proving the fan-out is concurrent
func TestFanOutDeliversConcurrently(t *testing.T) {
	var arrived sync.WaitGroup
	arrived.Add(2)
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arrived.Done()
		<-release
		w.WriteHeader(http.StatusOK)
	})
	a := httptest.NewServer(handler)
	defer a.Close()
	b := httptest.NewServer(handler)
	defer b.Close()

	targets := []Target{
		{Name: "a", HTTP: &HTTPTarget{URL: a.URL, Format: v1alpha1.PayloadFormatJSON, Client: a.Client()}},
		{Name: "b", HTTP: &HTTPTarget{URL: b.URL, Format: v1alpha1.PayloadFormatJSON, Client: b.Client()}},
	}
	go FanOut(context.Background(), targets, sampleEnvelope(), []byte("RAW"))

	both := make(chan struct{})
	go func() { arrived.Wait(); close(both) }()
	select {
	case <-both:
	case <-time.After(2 * time.Second):
		t.Fatal("destinations were not delivered concurrently")
	}
	close(release)
}

// Feature: fan-out delivery contract
// Scenario: a delivery records its metrics and clears the in-flight gauge
func TestFanOutRecordsDeliveryMetrics(t *testing.T) {
	deliveriesTotal.Reset()
	ok, closeOK := httpTarget(t, http.StatusOK)
	defer closeOK()

	FanOut(context.Background(), []Target{{Name: "metered", Required: true, HTTP: ok}}, sampleEnvelope(), []byte("RAW"))

	if got := testutil.ToFloat64(deliveriesTotal.WithLabelValues("metered", "http", "success")); got != 1 {
		t.Errorf("deliveries_total{success} = %v, want 1", got)
	}
	if inflight := testutil.ToFloat64(deliveriesInFlight); inflight != 0 {
		t.Errorf("deliveries_in_flight = %v, want 0 after delivery", inflight)
	}
}
