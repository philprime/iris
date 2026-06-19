/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/philprime/iris/api/v1alpha1"
)

func sampleEnvelope() *Envelope {
	return &Envelope{
		Version:        EnvelopeVersion,
		IdempotencyKey: "k1",
		Subject:        "Welcome",
		From:           "a@email.apple.com",
		To:             []string{"invites@invite.example.com"},
	}
}

// Feature: transform
// Scenario: the default json format emits the canonical envelope
func TestRenderJSONCanonical(t *testing.T) {
	body, contentType, err := Render(sampleEnvelope(), []byte("RAW"), v1alpha1.PayloadFormatJSON, "")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("contentType = %q, want application/json", contentType)
	}
	var got Envelope
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal canonical: %v", err)
	}
	if got.Subject != "Welcome" || got.IdempotencyKey != "k1" {
		t.Errorf("canonical envelope mismatch: %+v", got)
	}
}

// Feature: transform
// Scenario: the raw format forwards the original message bytes
func TestRenderRawPassesThrough(t *testing.T) {
	body, contentType, err := Render(sampleEnvelope(), []byte("RFC822-BYTES"), v1alpha1.PayloadFormatRaw, "")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if string(body) != "RFC822-BYTES" {
		t.Errorf("raw body = %q, want RFC822-BYTES", body)
	}
	if !strings.HasPrefix(contentType, "message/rfc822") {
		t.Errorf("contentType = %q, want message/rfc822", contentType)
	}
}

// Feature: transform
// Scenario: a Jsonnet program remaps the canonical envelope
//
//	Given a Jsonnet program reading the envelope ext var
//	When  the json payload is rendered
//	Then  the output is the remapped document
func TestRenderJsonnetRemap(t *testing.T) {
	program := `local e = std.extVar("envelope"); { mappedSubject: e.subject, recipient: e.to[0] }`
	body, _, err := Render(sampleEnvelope(), nil, v1alpha1.PayloadFormatJSON, program)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal remapped: %v", err)
	}
	if got["mappedSubject"] != "Welcome" {
		t.Errorf("mappedSubject = %v, want Welcome", got["mappedSubject"])
	}
	if got["recipient"] != "invites@invite.example.com" {
		t.Errorf("recipient = %v, want invites@invite.example.com", got["recipient"])
	}
}

// Feature: transform
// Scenario: an invalid Jsonnet program surfaces an error
func TestRenderJsonnetError(t *testing.T) {
	if _, _, err := Render(sampleEnvelope(), nil, v1alpha1.PayloadFormatJSON, "{ this is not valid"); err == nil {
		t.Error("expected error for invalid jsonnet")
	}
}
