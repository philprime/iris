/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/kula-app/go-health/core"
)

// lastReloadOK records whether the most recent postmap+reload succeeded. A
// failed reload means the ingress is serving stale routes, which should drain
// the replica. It starts true since no reload has been attempted yet.
var lastReloadOK atomic.Bool

func init() { lastReloadOK.Store(true) }

// newHealthEngine builds the reloader's health engine. Readiness fails when the
// Postfix master is unreachable or the last reload failed.
func newHealthEngine(logger *slog.Logger) *core.Engine {
	if logger == nil {
		logger = slog.Default()
	}
	eng := core.NewEngine("iris-reloader", "Iris Postfix reloader", core.WithLogger(logger))

	eng.RegisterReadinessCheck(core.Check{
		Name:          "postfix:status",
		ComponentType: "system",
		Timeout:       3 * time.Second,
		Run: func(ctx context.Context) []core.Result {
			if err := execRunner(ctx, "postfix", "status"); err != nil {
				return []core.Result{{Status: core.StatusFail, Output: err.Error()}}
			}
			return []core.Result{{Status: core.StatusPass}}
		},
	})

	eng.RegisterReadinessCheck(core.Check{
		Name:          "reload:last",
		ComponentType: "component",
		Run: func(_ context.Context) []core.Result {
			if !lastReloadOK.Load() {
				return []core.Result{{Status: core.StatusFail, Output: "last postfix reload failed"}}
			}
			return []core.Result{{Status: core.StatusPass}}
		},
	})

	return eng
}
