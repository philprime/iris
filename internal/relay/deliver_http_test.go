/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Feature: HTTP delivery
// Scenario: a delivery carries the idempotency key, auth, and body
//
//	Given an HTTP destination with a bearer token
//	When  a message is delivered
//	Then  the request method, Idempotency-Key, Authorization, and body are sent
func TestDeliverHTTPSendsRequest(t *testing.T) {
	var gotMethod, gotKey, gotAuth, gotBody, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotKey = r.Header.Get("Idempotency-Key")
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target := &HTTPTarget{URL: srv.URL, Method: "POST", AuthToken: "tok", Client: srv.Client()}
	err := deliverHTTP(context.Background(), target, "key-1", []byte(`{"a":1}`), "application/json")
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotKey != "key-1" {
		t.Errorf("Idempotency-Key = %q, want key-1", gotKey)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody != `{"a":1}` {
		t.Errorf("body = %q", gotBody)
	}
}

// Feature: HTTP delivery
// Scenario: a non-2xx response is a delivery failure
//
//	Given a destination returning 500
//	When  a message is delivered
//	Then  an error is returned so a required destination triggers a retry
func TestDeliverHTTPFailsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	target := &HTTPTarget{URL: srv.URL, Client: srv.Client()}
	if err := deliverHTTP(context.Background(), target, "k", []byte("{}"), "application/json"); err == nil {
		t.Error("expected error on 500 response")
	}
}
