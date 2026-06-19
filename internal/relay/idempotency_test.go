/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"testing"

	"github.com/philprime/iris/api/v1alpha1"
)

const messageWithID = "From: a@email.apple.com\r\n" +
	"Message-ID: <unique-123@email.apple.com>\r\n" +
	"Subject: hi\r\n\r\nbody\r\n"

// Feature: idempotency key
// Scenario: messageId mode uses the Message-ID header without angle brackets
func TestDeriveIdempotencyKeyMessageID(t *testing.T) {
	got := DeriveIdempotencyKey(v1alpha1.IdempotencyMessageID, []byte(messageWithID))
	if got != "unique-123@email.apple.com" {
		t.Errorf("key = %q, want unique-123@email.apple.com", got)
	}
}

// Feature: idempotency key
// Scenario: sha256 mode digests the whole message
func TestDeriveIdempotencyKeySHA256(t *testing.T) {
	got := DeriveIdempotencyKey(v1alpha1.IdempotencySHA256, []byte(messageWithID))
	if len(got) != 64 {
		t.Errorf("sha256 key length = %d, want 64 hex chars (%q)", len(got), got)
	}
	// Deterministic for the same input.
	if again := DeriveIdempotencyKey(v1alpha1.IdempotencySHA256, []byte(messageWithID)); again != got {
		t.Errorf("sha256 key not stable: %q vs %q", got, again)
	}
}

// Feature: idempotency key
// Scenario: messageId mode falls back to a digest when no Message-ID is present
//
//	Given a message without a Message-ID header
//	When  the key is derived in messageId mode
//	Then  a sha256 digest is used so a key always exists
func TestDeriveIdempotencyKeyFallsBackToSHA256(t *testing.T) {
	noID := []byte("From: a@email.apple.com\r\nSubject: hi\r\n\r\nbody\r\n")
	got := DeriveIdempotencyKey(v1alpha1.IdempotencyMessageID, noID)
	if len(got) != 64 {
		t.Errorf("fallback key length = %d, want 64 hex chars (%q)", len(got), got)
	}
}
