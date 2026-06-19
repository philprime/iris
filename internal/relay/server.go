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
	"log/slog"
	"strings"

	"github.com/emersion/go-smtp"

	"github.com/philprime/iris/api/v1alpha1"
)

// Backend is the go-smtp Backend that runs the relay pipeline for each inbound
// session: filter, transform, and fan out to the destinations.
type Backend struct {
	routes      []v1alpha1.Route
	filters     *v1alpha1.Filters
	idempotency v1alpha1.IdempotencyMode
	targets     []Target
	logger      *slog.Logger
}

// BackendConfig configures a relay Backend.
type BackendConfig struct {
	Routes      []v1alpha1.Route
	Filters     *v1alpha1.Filters
	Idempotency v1alpha1.IdempotencyMode
	Targets     []Target
}

// NewBackend builds a relay Backend from its config and resolved targets.
func NewBackend(cfg BackendConfig, logger *slog.Logger) *Backend {
	if logger == nil {
		logger = slog.Default()
	}
	return &Backend{
		routes:      cfg.Routes,
		filters:     cfg.Filters,
		idempotency: cfg.Idempotency,
		targets:     cfg.Targets,
		logger:      logger,
	}
}

// NewSession starts a new SMTP session.
func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &session{backend: b}, nil
}

// claims reports whether the relay accepts the recipient, matching an exact
// address route or a whole-domain route.
func (b *Backend) claims(recipient string) bool {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	domain := senderDomain(recipient)
	for _, route := range b.routes {
		if route.Address != "" && strings.ToLower(route.Address) == recipient {
			return true
		}
		if route.Domain != "" && strings.EqualFold(route.Domain, domain) {
			return true
		}
	}
	return false
}

// session carries the per-message SMTP state.
type session struct {
	backend *Backend
	from    string
	rcpts   []string
}

// Mail applies the sender hard rule before accepting the envelope sender.
func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if !SenderAllowed(s.backend.filters, from) {
		return &smtp.SMTPError{Code: 550, Message: "sender domain not allowed"}
	}
	s.from = from
	return nil
}

// Rcpt accepts only recipients the relay claims.
func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if !s.backend.claims(to) {
		return &smtp.SMTPError{Code: 550, Message: "relay access denied for recipient"}
	}
	s.rcpts = append(s.rcpts, to)
	return nil
}

// Data runs the filter, transform, and fan-out pipeline and reflects the result
// as the SMTP response: 250 when all required destinations succeed, 4xx when a
// required destination fails so Postfix retries.
func (s *session) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return &smtp.SMTPError{Code: 451, Message: "could not read message"}
	}

	decision := Evaluate(s.backend.filters, s.from, raw, int64(len(raw)))
	if !decision.Accept {
		return rejectionError(decision.Reason)
	}

	key := DeriveIdempotencyKey(s.backend.idempotency, raw)
	env, err := BuildEnvelope(s.from, s.rcpts, raw, key)
	if err != nil {
		s.backend.logger.Error("build envelope", slog.Any("error", err))
		return &smtp.SMTPError{Code: 451, Message: "could not parse message"}
	}

	result := FanOut(context.Background(), s.backend.targets, env, raw)
	for name, derr := range result.Results {
		if derr != nil {
			s.backend.logger.Warn("destination delivery failed", slog.String("destination", name), slog.Any("error", derr))
		}
	}
	if result.RequiredFailed {
		return &smtp.SMTPError{Code: 451, Message: "a required destination failed, retry later"}
	}
	return nil
}

// Reset clears the per-message state.
func (s *session) Reset() {
	s.from = ""
	s.rcpts = nil
}

// Logout releases the session.
func (s *session) Logout() error { return nil }

// rejectionError maps a filter rejection reason to an SMTP error: an oversize
// message is 552, every other hard or score failure is 550.
func rejectionError(reason string) error {
	if reason == ReasonSize {
		return &smtp.SMTPError{Code: 552, Message: "message exceeds maximum size"}
	}
	return &smtp.SMTPError{Code: 550, Message: "message rejected by filter: " + reason}
}
