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
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
)

// captureSession records the envelope and body of one delivered message.
type captureSession struct {
	be *captureBackend
}

func (s *captureSession) Mail(from string, _ *smtp.MailOptions) error {
	s.be.from = from
	return nil
}

func (s *captureSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.be.to = append(s.be.to, to)
	return nil
}

func (s *captureSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	s.be.data = string(b)
	close(s.be.received)
	return err
}

func (s *captureSession) Reset()        {}
func (s *captureSession) Logout() error { return nil }

// captureBackend accepts a single message and records it.
type captureBackend struct {
	from     string
	to       []string
	data     string
	received chan struct{}
}

func (b *captureBackend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &captureSession{be: b}, nil
}

// Feature: SMTP delivery
// Scenario: a message is forwarded to a downstream SMTP server
//
//	Given a downstream SMTP server
//	When  a raw message is delivered
//	Then  the server receives the envelope sender, recipient, and body
func TestDeliverSMTPForwardsMessage(t *testing.T) {
	be := &captureBackend{received: make(chan struct{})}
	server := smtp.NewServer(be)
	server.AllowInsecureAuth = true

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	host, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	target := &SMTPTarget{Host: host, Port: port}

	raw := []byte("From: a@email.apple.com\r\nTo: invites@invite.example.com\r\nSubject: hi\r\n\r\nbody\r\n")
	if err := deliverSMTP(context.Background(), target, "a@email.apple.com", []string{"invites@invite.example.com"}, raw); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	select {
	case <-be.received:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the message")
	}

	if be.from != "a@email.apple.com" {
		t.Errorf("from = %q", be.from)
	}
	if len(be.to) != 1 || be.to[0] != "invites@invite.example.com" {
		t.Errorf("to = %v", be.to)
	}
	if !strings.Contains(be.data, "Subject: hi") || !strings.Contains(be.data, "body") {
		t.Errorf("data = %q", be.data)
	}
}
