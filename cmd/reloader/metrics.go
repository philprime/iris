/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import "github.com/prometheus/client_golang/prometheus"

// The reloader's iris_postfix_* collectors register with the default Prometheus
// registry so they serve on the admin server's /metrics.
var (
	reloadsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "iris", Subsystem: "postfix", Name: "reloads_total",
		Help: "postfix reload attempts by result.",
	}, []string{"result"})

	reloadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "iris", Subsystem: "postfix", Name: "reload_duration_seconds",
		Help:    "Reload latency.",
		Buckets: prometheus.DefBuckets,
	})

	lastReloadTimestamp = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "iris", Subsystem: "postfix", Name: "config_last_reload_timestamp_seconds",
		Help: "Unix time of the last successful reload.",
	})
)

func init() {
	prometheus.MustRegister(reloadsTotal, reloadDuration, lastReloadTimestamp)
}
