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
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/emersion/go-smtp"

	"github.com/philprime/iris/api/v1alpha1"
)

// startRelay runs the given backend as a go-smtp server on a random port and
// returns an SMTPTarget pointing at it.
func startRelay(t *testing.T, backend *Backend) *SMTPTarget {
	t.Helper()
	server := smtp.NewServer(backend)
	server.AllowInsecureAuth = true
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	host, portStr, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return &SMTPTarget{Host: host, Port: port}
}

const inboundMessage = "From: noreply@email.apple.com\r\n" +
	"To: invites@invite.example.com\r\n" +
	"Subject: hi\r\n\r\nhello\r\n"

// Feature: relay session pipeline
// Scenario: a claimed message is filtered, transformed, and delivered
//
//	Given a relay claiming a recipient with one HTTP destination
//	When  a message is sent to that recipient over SMTP
//	Then  the relay accepts it and the destination receives the delivery
func TestRelayDeliversClaimedMessage(t *testing.T) {
	delivered := make(chan string, 1)
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		delivered <- string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()

	backend := NewBackend(BackendConfig{
		Routes:  []v1alpha1.Route{{Domain: "invite.example.com"}},
		Targets: []Target{{Name: "webhook", Required: true, HTTP: &HTTPTarget{URL: dest.URL, Format: v1alpha1.PayloadFormatJSON, Client: dest.Client()}}},
	}, nil)
	relay := startRelay(t, backend)

	err := deliverSMTP(context.Background(), relay, "noreply@email.apple.com", []string{"invites@invite.example.com"}, []byte(inboundMessage))
	if err != nil {
		t.Fatalf("send to relay: %v", err)
	}

	select {
	case body := <-delivered:
		if body == "" {
			t.Error("destination received an empty body")
		}
	default:
		t.Error("destination did not receive the delivery")
	}
}

// Feature: relay session pipeline
// Scenario: a recipient the relay does not claim is rejected
//
//	Given a relay claiming only invite.example.com
//	When  a message is sent to an unclaimed recipient
//	Then  the relay rejects it at RCPT TO
func TestRelayRejectsUnclaimedRecipient(t *testing.T) {
	backend := NewBackend(BackendConfig{
		Routes: []v1alpha1.Route{{Domain: "invite.example.com"}},
	}, nil)
	relay := startRelay(t, backend)

	err := deliverSMTP(context.Background(), relay, "noreply@email.apple.com", []string{"someone@other.example"}, []byte(inboundMessage))
	if err == nil {
		t.Fatal("expected the relay to reject an unclaimed recipient")
	}
}
