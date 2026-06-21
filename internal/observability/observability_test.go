/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"

	"github.com/philprime/iris/internal/config"
)

// Feature: Sentry trace sampling
// Scenario: probe and metrics spans are never traced
//
//	Given a sampler at a non-zero rate
//	When  a /healthz span is sampled
//	Then  the rate is 0 so health traffic does not flood the trace quota
func TestTracesSamplerDropsProbeSpans(t *testing.T) {
	sampler := TracesSampler(config.Sentry{TracesSampleRate: 0.5})

	if rate := sampler(sentry.SamplingContext{Span: &sentry.Span{Name: "GET /healthz"}}); rate != 0.0 {
		t.Errorf("probe span sampled at %v, want 0", rate)
	}
	if rate := sampler(sentry.SamplingContext{Span: &sentry.Span{Name: "reconcile Relay"}}); rate != 0.5 {
		t.Errorf("non-probe span sampled at %v, want 0.5", rate)
	}
}

// Feature: Sentry release identifier
// Scenario: the unified philprime release format is used
func TestReleaseID(t *testing.T) {
	if got := ReleaseID("1.2.3", "abc123"); got != "iris@1.2.3:abc123" {
		t.Errorf("ReleaseID = %q, want iris@1.2.3:abc123", got)
	}
}

// Feature: Sentry release resolution
// Scenario: the injected release wins, otherwise it is derived
//
//	Given an injected release that is set or empty
//	When  ResolveRelease picks the release
//	Then  a set value is returned verbatim and an empty one falls back to
//	      iris@<version>:<commit>
func TestResolveRelease(t *testing.T) {
	if got := ResolveRelease("iris@9.9.9:deadbee", "dev", "none"); got != "iris@9.9.9:deadbee" {
		t.Errorf("ResolveRelease(injected) = %q, want iris@9.9.9:deadbee", got)
	}
	if got := ResolveRelease("", "dev", "none"); got != "iris@dev:none" {
		t.Errorf("ResolveRelease(empty) = %q, want iris@dev:none", got)
	}
}

// Feature: observability setup
// Scenario: with Sentry disabled the logger is terminal-only
//
//	Given Sentry disabled
//	When  the logger is built and a message logged
//	Then  the message reaches the terminal handler and flush is a no-op
func TestSetupDisabledReturnsTerminalLogger(t *testing.T) {
	var buf bytes.Buffer
	logger, flush := Setup(context.Background(), config.Sentry{Enabled: false}, slog.NewTextHandler(&buf, nil))
	defer flush()

	logger.Info("hello-terminal")

	if !strings.Contains(buf.String(), "hello-terminal") {
		t.Errorf("terminal handler missing record: %q", buf.String())
	}
}
