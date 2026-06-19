/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kula-app/go-health/core"
)

// NewHealthEngine builds the relay's health engine. Readiness gates only what
// blocks accepting SMTP (the listener being bound). Destination reachability is
// a healthz-only, informational check, never a readiness gate, because Postfix
// queues and retries on failure so a flaky downstream must not drain the relay.
func NewHealthEngine(smtpAddr string, targets []Target, logger *slog.Logger) *core.Engine {
	if logger == nil {
		logger = slog.Default()
	}
	eng := core.NewEngine("iris-relay", "Iris relay data plane", core.WithLogger(logger))

	eng.RegisterReadinessCheck(core.Check{
		Name:          "smtp:listener",
		ComponentType: "component",
		Timeout:       time.Second,
		Run: func(ctx context.Context) []core.Result {
			conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", dialAddr(smtpAddr))
			if err != nil {
				return []core.Result{{Status: core.StatusFail, Output: err.Error()}}
			}
			_ = conn.Close()
			return []core.Result{{Status: core.StatusPass}}
		},
	})

	for i := range targets {
		target := targets[i]
		eng.RegisterHealthCheck(core.Check{
			Name:          "destination:" + target.Name,
			ComponentType: "component",
			Timeout:       2 * time.Second,
			Run: func(ctx context.Context) []core.Result {
				return destinationReachable(ctx, &target)
			},
		})
	}

	return eng
}

// destinationReachable performs a cheap reachability probe: a HEAD for HTTP
// destinations (any response means reachable) and a dial for SMTP.
func destinationReachable(ctx context.Context, target *Target) []core.Result {
	switch {
	case target.HTTP != nil:
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, target.HTTP.URL, nil)
		if err != nil {
			return []core.Result{{Status: core.StatusFail, Output: err.Error()}}
		}
		client := target.HTTP.Client
		if client == nil {
			client = http.DefaultClient
		}
		resp, err := client.Do(req)
		if err != nil {
			return []core.Result{{Status: core.StatusFail, Output: err.Error()}}
		}
		_ = resp.Body.Close()
		return []core.Result{{Status: core.StatusPass}}
	case target.SMTP != nil:
		addr := net.JoinHostPort(target.SMTP.Host, strconv.Itoa(target.SMTP.Port))
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return []core.Result{{Status: core.StatusFail, Output: err.Error()}}
		}
		_ = conn.Close()
		return []core.Result{{Status: core.StatusPass}}
	default:
		return []core.Result{{Status: core.StatusFail, Output: "destination has no delivery method"}}
	}
}

// dialAddr turns a bind address like ":25" into a dialable "127.0.0.1:25".
func dialAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
