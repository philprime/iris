/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// SMTPTarget is a resolved downstream SMTP destination. Pointing it at another
// relay's Service is how manual relay-to-relay chaining works.
type SMTPTarget struct {
	Host     string
	Port     int
	StartTLS bool
	Username string
	Password string
}

// deliverSMTP forwards the raw message to the downstream SMTP server, optionally
// upgrading with STARTTLS and authenticating with PLAIN credentials.
func deliverSMTP(_ context.Context, t *SMTPTarget, from string, to []string, raw []byte) error {
	addr := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))

	var (
		client *smtp.Client
		err    error
	)
	if t.StartTLS {
		client, err = smtp.DialStartTLS(addr, &tls.Config{ServerName: t.Host})
	} else {
		client, err = smtp.Dial(addr)
	}
	if err != nil {
		return fmt.Errorf("dial smtp %s: %w", addr, err)
	}
	defer client.Close()

	if t.Username != "" {
		if err := client.Auth(sasl.NewPlainClient("", t.Username, t.Password)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.SendMail(from, to, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}
