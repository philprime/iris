/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-message/mail"
)

// EnvelopeVersion is the schema version of the canonical envelope. It is part
// of the public delivery contract and what a Jsonnet transform receives.
const EnvelopeVersion = "v1"

// Envelope is the canonical representation of an inbound message: a fixed,
// versioned schema that every destination receives (as JSON) and that a Jsonnet
// transform remaps.
type Envelope struct {
	Version        string            `json:"version"`
	IdempotencyKey string            `json:"idempotencyKey"`
	Envelope       SMTPEnvelope      `json:"envelope"`
	Headers        map[string]string `json:"headers"`
	From           string            `json:"from"`
	To             []string          `json:"to"`
	Subject        string            `json:"subject"`
	Text           string            `json:"text"`
	HTML           string            `json:"html"`
	Attachments    []Attachment      `json:"attachments"`
	Raw            string            `json:"raw,omitempty"`
}

// SMTPEnvelope carries the SMTP-level sender and recipients, which are
// independent of the message headers.
type SMTPEnvelope struct {
	MailFrom string   `json:"mailFrom"`
	RcptTo   []string `json:"rcptTo"`
}

// Attachment is a non-inline message part captured as base64 bytes.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	BytesBase64 string `json:"bytesBase64"`
}

// BuildEnvelope parses a raw RFC822 message into the canonical envelope, using
// the SMTP-level sender, recipients, and precomputed idempotency key.
func BuildEnvelope(mailFrom string, rcptTo []string, raw []byte, idempotencyKey string) (*Envelope, error) {
	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}

	env := &Envelope{
		Version:        EnvelopeVersion,
		IdempotencyKey: idempotencyKey,
		Envelope:       SMTPEnvelope{MailFrom: mailFrom, RcptTo: rcptTo},
		Headers:        map[string]string{},
	}

	fields := reader.Header.Fields()
	for fields.Next() {
		if _, seen := env.Headers[fields.Key()]; !seen {
			env.Headers[fields.Key()] = fields.Value()
		}
	}
	if froms, err := reader.Header.AddressList("From"); err == nil && len(froms) > 0 {
		env.From = froms[0].Address
	}
	if tos, err := reader.Header.AddressList("To"); err == nil {
		for _, addr := range tos {
			env.To = append(env.To, addr.Address)
		}
	}
	if subject, err := reader.Header.Subject(); err == nil {
		env.Subject = subject
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read message part: %w", err)
		}
		body, err := io.ReadAll(part.Body)
		if err != nil {
			return nil, fmt.Errorf("read part body: %w", err)
		}
		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := header.ContentType()
			switch {
			case strings.HasPrefix(contentType, "text/plain"):
				env.Text = string(body)
			case strings.HasPrefix(contentType, "text/html"):
				env.HTML = string(body)
			}
		case *mail.AttachmentHeader:
			filename, _ := header.Filename()
			contentType, _, _ := header.ContentType()
			env.Attachments = append(env.Attachments, Attachment{
				Filename:    filename,
				ContentType: contentType,
				BytesBase64: base64.StdEncoding.EncodeToString(body),
			})
		}
	}

	return env, nil
}
