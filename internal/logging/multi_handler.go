/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package logging provides the shared slog wiring used by every Iris binary,
// chiefly a MultiHandler that fans each record out to several handlers (for
// example a terminal handler and the Sentry handler).
package logging

import (
	"context"
	"errors"
	"log/slog"
)

// MultiHandler is an slog.Handler that forwards every record to each of its
// child handlers, so a single logger can write to the terminal and Sentry at
// once.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler returns a handler that fans out to all of the given handlers.
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

// Enabled reports whether any child handler is enabled for the level.
func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, child := range h.handlers {
		if child.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle forwards the record to every child that is enabled for its level,
// joining any errors so one failing handler does not hide the others.
func (h *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, child := range h.handlers {
		if !child.Enabled(ctx, record.Level) {
			continue
		}
		if err := child.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// WithAttrs returns a handler whose children all carry the given attributes.
func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		next[i] = child.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: next}
}

// WithGroup returns a handler whose children are all grouped under name.
func (h *MultiHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		next[i] = child.WithGroup(name)
	}
	return &MultiHandler{handlers: next}
}
