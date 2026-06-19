/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package observability

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// recordingTransport captures the events the Sentry client would have sent.
type recordingTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *recordingTransport) Configure(sentry.ClientOptions)        {}
func (t *recordingTransport) Flush(time.Duration) bool              { return true }
func (t *recordingTransport) FlushWithContext(context.Context) bool { return true }
func (t *recordingTransport) Close()                                {}
func (t *recordingTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

// Feature: error reporting
// Scenario: an error is captured with its tags
//
//	Given Sentry initialized with a recording transport
//	When  CaptureError is called with tags
//	Then  one event is sent carrying the exception message and the tags
func TestCaptureErrorTagsAndSends(t *testing.T) {
	transport := &recordingTransport{}
	if err := sentry.Init(sentry.ClientOptions{Dsn: "http://test@localhost/1", Transport: transport}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	t.Cleanup(func() { _ = sentry.Init(sentry.ClientOptions{}) })

	CaptureError(context.Background(), errors.New("terminal boom"), map[string]string{
		"relay.namespace": "default",
		"relay.name":      "alpha",
	})
	sentry.Flush(time.Second)

	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.events) != 1 {
		t.Fatalf("captured %d events, want 1", len(transport.events))
	}
	event := transport.events[0]
	if event.Tags["relay.name"] != "alpha" || event.Tags["relay.namespace"] != "default" {
		t.Errorf("tags = %v, want relay.namespace=default relay.name=alpha", event.Tags)
	}
	if len(event.Exception) == 0 || !strings.Contains(event.Exception[len(event.Exception)-1].Value, "terminal boom") {
		t.Errorf("exception = %+v, want one containing 'terminal boom'", event.Exception)
	}
}

// Feature: error reporting
// Scenario: a nil error is not captured
func TestCaptureErrorIgnoresNil(t *testing.T) {
	transport := &recordingTransport{}
	if err := sentry.Init(sentry.ClientOptions{Dsn: "http://test@localhost/1", Transport: transport}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	t.Cleanup(func() { _ = sentry.Init(sentry.ClientOptions{}) })

	CaptureError(context.Background(), nil, nil)
	sentry.Flush(time.Second)

	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.events) != 0 {
		t.Errorf("captured %d events for a nil error, want 0", len(transport.events))
	}
}
