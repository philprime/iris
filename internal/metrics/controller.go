/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package metrics declares the iris_* Prometheus collectors. The controller's
// collectors register with the controller-runtime registry so they serve on the
// manager's metrics endpoint. Data-plane collectors (relay, reloader) use the
// default registry and are declared alongside their binaries.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const namespace = "iris"

var (
	// Relays counts relays by status phase (ready, conflict, programming).
	Relays = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "relays",
		Help:      "Number of Relays by status phase.",
	}, []string{"phase"})

	// RouteConflicts counts route keys currently losing first-writer-wins.
	RouteConflicts = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "route_conflicts",
		Help:      "Number of routes currently losing first-writer-wins conflict resolution.",
	})

	// PostfixConfigRenders counts Postfix-map render-and-write attempts by
	// result (written, nochange, error).
	PostfixConfigRenders = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "postfix_config_renders_total",
		Help:      "Postfix map render-and-write attempts by result.",
	}, []string{"result"})

	// PostfixConfigGeneration is the generation of the last written Postfix
	// maps ConfigMap.
	PostfixConfigGeneration = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "postfix_config_generation",
		Help:      "Generation of the last written Postfix maps ConfigMap.",
	})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		Relays,
		RouteConflicts,
		PostfixConfigRenders,
		PostfixConfigGeneration,
	)
}
