/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import "github.com/prometheus/client_golang/prometheus"

// The relay's iris_relay_* collectors register with the default Prometheus
// registry so they serve on the admin server's /metrics. They live here rather
// than in internal/metrics so the relay binary does not import controller-runtime.
var (
	sessionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "iris", Subsystem: "relay", Name: "sessions_total",
		Help: "SMTP sessions by result.",
	}, []string{"result"})

	messagesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "iris", Subsystem: "relay", Name: "messages_total",
		Help: "Messages by outcome.",
	}, []string{"result"})

	filterScore = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "iris", Subsystem: "relay", Name: "filter_score",
		Help:    "Heuristic filter score distribution.",
		Buckets: prometheus.LinearBuckets(0, 1, 11),
	})

	messageBytes = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "iris", Subsystem: "relay", Name: "message_bytes",
		Help:    "Accepted message size in bytes.",
		Buckets: prometheus.ExponentialBuckets(1024, 2, 16),
	})

	deliveriesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "iris", Subsystem: "relay", Name: "deliveries_total",
		Help: "Per-destination fan-out deliveries by type and result.",
	}, []string{"destination", "type", "result"})

	deliveryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "iris", Subsystem: "relay", Name: "delivery_duration_seconds",
		Help:    "Per-destination delivery latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"destination", "type"})

	deliveriesInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "iris", Subsystem: "relay", Name: "deliveries_in_flight",
		Help: "Concurrent in-flight deliveries.",
	})
)

func init() {
	prometheus.MustRegister(
		sessionsTotal,
		messagesTotal,
		filterScore,
		messageBytes,
		deliveriesTotal,
		deliveryDuration,
		deliveriesInFlight,
	)
}

// targetType returns the metric "type" label for a target.
func targetType(t *Target) string {
	if t.SMTP != nil {
		return "smtp"
	}
	return "http"
}
