/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package observability wires Sentry error reporting into slog. It is shared by
// every Iris binary: Setup builds a logger that fans terminal output and Sentry
// together, and returns a flush function to defer on shutdown.
package observability

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentryslog "github.com/getsentry/sentry-go/slog"
	"github.com/go-logr/logr"

	"github.com/philprime/iris/internal/config"
	"github.com/philprime/iris/internal/logging"
)

// flushTimeout bounds how long Setup's flush waits to deliver buffered events.
const flushTimeout = 2 * time.Second

// Setup initializes Sentry when enabled and returns a logger plus a flush
// function. With Sentry disabled the logger writes only to the terminal handler
// and flush is a no-op, so local and air-gapped installs run clean.
func Setup(ctx context.Context, cfg config.Sentry, terminal slog.Handler) (*slog.Logger, func()) {
	if !cfg.Enabled {
		return slog.New(terminal), func() {}
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		Debug:            cfg.Debug,
		AttachStacktrace: cfg.AttachStacktrace,
		SampleRate:       cfg.SampleRate,
		EnableTracing:    cfg.EnableTracing,
		TracesSampleRate: cfg.TracesSampleRate,
		TracesSampler:    TracesSampler(cfg),
	})
	if err != nil {
		logger := slog.New(terminal)
		logger.ErrorContext(ctx, "initialize sentry, continuing with terminal logging only", slog.Any("error", err))
		return logger, func() {}
	}

	sentryHandler := sentryslog.Option{
		EventLevel: []slog.Level{slog.LevelError},
		LogLevel:   []slog.Level{slog.LevelWarn, slog.LevelInfo},
	}.NewSentryHandler(ctx)

	logger := slog.New(logging.NewMultiHandler(sentryHandler, terminal))
	return logger, func() { sentry.Flush(flushTimeout) }
}

// TracesSampler drops probe and metrics spans (sample rate 0) so health traffic
// never floods the trace quota, and samples everything else at the configured
// rate. It is wired regardless of whether tracing is on, so enabling tracing
// later never traces probes.
func TracesSampler(cfg config.Sentry) sentry.TracesSampler {
	return func(samplingCtx sentry.SamplingContext) float64 {
		if samplingCtx.Span != nil && isProbeSpan(samplingCtx.Span.Name) {
			return 0.0
		}
		return cfg.TracesSampleRate
	}
}

func isProbeSpan(name string) bool {
	for _, probe := range []string{"/healthz", "/livez", "/readyz", "/metrics"} {
		if strings.Contains(name, probe) {
			return true
		}
	}
	return false
}

// LogrFromSlog adapts an slog.Logger into the logr.Logger controller-runtime
// expects, so manager and reconciler logs flow through the same pipeline.
func LogrFromSlog(logger *slog.Logger) logr.Logger {
	return logr.FromSlogHandler(logger.Handler())
}

// ReleaseID builds the unified philprime Sentry release identifier
// iris@<version>:<git-sha>.
func ReleaseID(version, gitSHA string) string {
	return fmt.Sprintf("iris@%s:%s", version, gitSHA)
}

// ResolveRelease picks the Sentry release identifier: the ldflags-injected
// value when set, otherwise one derived from the build version and commit.
func ResolveRelease(injected, version, commit string) string {
	if injected != "" {
		return injected
	}
	return ReleaseID(version, commit)
}

// CaptureError reports an error to Sentry with the given tags attached to a
// fresh scope, following the philprime house style: prefer the context-scoped
// hub, fall back to the current hub, and attach context on a WithScope closure
// so tags never leak across concurrent reconciles. It is a no-op when Sentry is
// not initialized. Callers should capture only unexpected (terminal) errors,
// not routine retries.
func CaptureError(ctx context.Context, err error, tags map[string]string) {
	if err == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		for key, value := range tags {
			scope.SetTag(key, value)
		}
		hub.CaptureException(err)
	})
}
