/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package adminserver builds the HTTP server that data-plane binaries expose to
// Kubernetes probes and Prometheus. It serves the go-health liveness,
// readiness, and health endpoints for a health engine plus /metrics from the
// default Prometheus registry.
package adminserver

import (
	"context"
	"net/http"
	"time"

	healthhttp "github.com/kula-app/go-health/adapters/http"
	"github.com/kula-app/go-health/core"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// shutdownTimeout bounds how long Shutdown waits for in-flight requests.
const shutdownTimeout = 5 * time.Second

// New returns an admin HTTP server bound to addr that serves the health
// endpoints for eng and Prometheus metrics from the default registry.
func New(addr string, eng *core.Engine) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/livez", healthhttp.LivezHandler(eng))
	mux.Handle("/readyz", healthhttp.ReadyzHandler(eng))
	mux.Handle("/healthz", healthhttp.HealthzHandler(eng))
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

// Shutdown gracefully stops server, waiting up to shutdownTimeout for in-flight
// requests to finish.
func Shutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = server.Shutdown(ctx)
}
