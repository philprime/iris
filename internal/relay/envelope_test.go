/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"encoding/base64"
	"strings"
	"testing"
)

const multipartMessage = "From: Alice <alice@email.apple.com>\r\n" +
	"To: invites@invite.example.com\r\n" +
	"Subject: Hello there\r\n" +
	"Message-ID: <abc123@email.apple.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/alternative; boundary=BOUND\r\n" +
	"\r\n" +
	"--BOUND\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"hello in plain text\r\n" +
	"--BOUND\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>hello in html</p>\r\n" +
	"--BOUND--\r\n"

// Feature: canonical envelope
// Scenario: a multipart message maps to the canonical envelope
//
//	Given a multipart/alternative message with text and html parts
//	When  the envelope is built
//	Then  the headers, addresses, subject, and both bodies are populated
func TestBuildEnvelopeFromMultipart(t *testing.T) {
	env, err := BuildEnvelope("alice@email.apple.com", []string{"invites@invite.example.com"}, []byte(multipartMessage), "abc123")
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}

	if env.Version != EnvelopeVersion {
		t.Errorf("version = %q, want %q", env.Version, EnvelopeVersion)
	}
	if env.IdempotencyKey != "abc123" {
		t.Errorf("idempotencyKey = %q, want abc123", env.IdempotencyKey)
	}
	if env.Envelope.MailFrom != "alice@email.apple.com" {
		t.Errorf("envelope.mailFrom = %q", env.Envelope.MailFrom)
	}
	if len(env.Envelope.RcptTo) != 1 || env.Envelope.RcptTo[0] != "invites@invite.example.com" {
		t.Errorf("envelope.rcptTo = %v", env.Envelope.RcptTo)
	}
	if env.From != "alice@email.apple.com" {
		t.Errorf("from = %q, want alice@email.apple.com", env.From)
	}
	if len(env.To) != 1 || env.To[0] != "invites@invite.example.com" {
		t.Errorf("to = %v", env.To)
	}
	if env.Subject != "Hello there" {
		t.Errorf("subject = %q, want Hello there", env.Subject)
	}
	if !strings.Contains(env.Text, "hello in plain text") {
		t.Errorf("text = %q", env.Text)
	}
	if !strings.Contains(env.HTML, "hello in html") {
		t.Errorf("html = %q", env.HTML)
	}
	if env.Headers["Subject"] != "Hello there" {
		t.Errorf("headers[Subject] = %q", env.Headers["Subject"])
	}
}

// Feature: canonical envelope
// Scenario: an attachment is captured as base64
//
//	Given a message with a text body and a file attachment
//	When  the envelope is built
//	Then  the attachment filename, content type, and base64 bytes are captured
func TestBuildEnvelopeCapturesAttachment(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte("PDF-BYTES"))
	msg := "From: bob@email.apple.com\r\n" +
		"To: invites@invite.example.com\r\n" +
		"Subject: doc\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=MIX\r\n" +
		"\r\n" +
		"--MIX\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"see attached\r\n" +
		"--MIX\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"report.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" + payload + "\r\n" +
		"--MIX--\r\n"

	env, err := BuildEnvelope("bob@email.apple.com", []string{"invites@invite.example.com"}, []byte(msg), "k")
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	if len(env.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(env.Attachments))
	}
	a := env.Attachments[0]
	if a.Filename != "report.pdf" {
		t.Errorf("filename = %q, want report.pdf", a.Filename)
	}
	if !strings.HasPrefix(a.ContentType, "application/pdf") {
		t.Errorf("contentType = %q, want application/pdf", a.ContentType)
	}
	if decoded, _ := base64.StdEncoding.DecodeString(a.BytesBase64); string(decoded) != "PDF-BYTES" {
		t.Errorf("attachment bytes = %q, want PDF-BYTES", decoded)
	}
}
