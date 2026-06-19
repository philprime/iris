/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/mail"
	"strings"

	"github.com/philprime/iris/api/v1alpha1"
)

// DeriveIdempotencyKey computes the stable key sent to every destination. In
// messageId mode it uses the Message-ID header, falling back to a SHA-256
// digest when the header is absent so a key always exists. In sha256 mode it
// always digests the whole message.
func DeriveIdempotencyKey(mode v1alpha1.IdempotencyMode, raw []byte) string {
	if mode != v1alpha1.IdempotencySHA256 {
		if id := messageID(raw); id != "" {
			return id
		}
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// messageID returns the Message-ID header value with surrounding angle brackets
// and whitespace trimmed, or empty when it cannot be read.
func messageID(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return strings.Trim(msg.Header.Get("Message-ID"), "<> ")
}
