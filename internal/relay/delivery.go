/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"context"
	"fmt"
)

// Target is a resolved destination ready to deliver: exactly one of HTTP or
// SMTP is set, along with whether its failure gates a retry.
type Target struct {
	Name     string
	Required bool
	HTTP     *HTTPTarget
	SMTP     *SMTPTarget
}

// Deliver renders and delivers the message to this destination. HTTP
// destinations send the rendered payload, SMTP destinations forward the raw
// message.
func (t *Target) Deliver(ctx context.Context, env *Envelope, raw []byte) error {
	switch {
	case t.HTTP != nil:
		body, contentType, err := Render(env, raw, t.HTTP.Format, t.HTTP.Jsonnet)
		if err != nil {
			return fmt.Errorf("render %q: %w", t.Name, err)
		}
		return deliverHTTP(ctx, t.HTTP, env.IdempotencyKey, body, contentType)
	case t.SMTP != nil:
		return deliverSMTP(ctx, t.SMTP, env.Envelope.MailFrom, env.Envelope.RcptTo, raw)
	default:
		return fmt.Errorf("destination %q has no delivery method", t.Name)
	}
}

// FanOutResult summarizes a fan-out: the per-destination outcomes and whether
// any required destination failed (which makes the session return SMTP 4xx so
// Postfix retries).
type FanOutResult struct {
	RequiredFailed bool
	Results        map[string]error
}

// FanOut delivers the message to every destination. Fan-out is a broadcast and
// is not atomic: a retry re-delivers to destinations that already succeeded, so
// every delivery carries the idempotency key for downstream dedup.
func FanOut(ctx context.Context, targets []Target, env *Envelope, raw []byte) FanOutResult {
	result := FanOutResult{Results: make(map[string]error, len(targets))}
	for i := range targets {
		target := &targets[i]
		err := target.Deliver(ctx, env, raw)
		result.Results[target.Name] = err
		if err != nil && target.Required {
			result.RequiredFailed = true
		}
	}
	return result
}
