/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// Feature: log fan-out
// Scenario: a record reaches every child handler
//
//	Given a MultiHandler wrapping two text handlers
//	When  a message is logged through it
//	Then  both handlers receive the message
func TestMultiHandlerFansOutToEveryHandler(t *testing.T) {
	var a, b bytes.Buffer
	logger := slog.New(NewMultiHandler(
		slog.NewTextHandler(&a, nil),
		slog.NewTextHandler(&b, nil),
	))

	logger.Info("hello", slog.String("k", "v"))

	for name, buf := range map[string]*bytes.Buffer{"a": &a, "b": &b} {
		if !strings.Contains(buf.String(), "hello") || !strings.Contains(buf.String(), "k=v") {
			t.Errorf("handler %s missing record: %q", name, buf.String())
		}
	}
}

// Feature: log fan-out
// Scenario: attributes propagate to every child handler
//
//	Given a MultiHandler with a derived logger carrying an attribute
//	When  a message is logged
//	Then  the attribute appears in every handler's output
func TestMultiHandlerWithAttrsPropagates(t *testing.T) {
	var a, b bytes.Buffer
	logger := slog.New(NewMultiHandler(
		slog.NewTextHandler(&a, nil),
		slog.NewTextHandler(&b, nil),
	)).With(slog.String("component", "relay"))

	logger.Info("ping")

	for name, buf := range map[string]*bytes.Buffer{"a": &a, "b": &b} {
		if !strings.Contains(buf.String(), "component=relay") {
			t.Errorf("handler %s missing propagated attr: %q", name, buf.String())
		}
	}
}

// Feature: log fan-out
// Scenario: enablement is the union of the children
//
//	Given one child that only accepts errors and one that accepts info
//	When  enablement is checked at info level
//	Then  the MultiHandler reports enabled because at least one child is
func TestMultiHandlerEnabledIsUnion(t *testing.T) {
	errOnly := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})
	infoOK := slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelInfo})
	h := NewMultiHandler(errOnly, infoOK)

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected MultiHandler enabled at info when one child accepts info")
	}
}
