/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Feature: controller domain metrics
// Scenario: the postfix render counter records outcomes by result
//
//	Given the iris_postfix_config_renders_total counter
//	When  a written render is recorded
//	Then  the written series increments
func TestPostfixConfigRendersCounts(t *testing.T) {
	PostfixConfigRenders.Reset()
	PostfixConfigRenders.WithLabelValues("written").Inc()
	if got := testutil.ToFloat64(PostfixConfigRenders.WithLabelValues("written")); got != 1 {
		t.Errorf("written render count = %v, want 1", got)
	}
}

// Feature: controller domain metrics
// Scenario: the postfix generation gauge tracks the last written generation
func TestPostfixConfigGenerationSets(t *testing.T) {
	PostfixConfigGeneration.Set(7)
	if got := testutil.ToFloat64(PostfixConfigGeneration); got != 7 {
		t.Errorf("postfix config generation = %v, want 7", got)
	}
}

// Feature: controller domain metrics
// Scenario: relays are counted by phase
func TestRelaysGaugeByPhase(t *testing.T) {
	Relays.Reset()
	Relays.WithLabelValues("ready").Set(3)
	if got := testutil.ToFloat64(Relays.WithLabelValues("ready")); got != 3 {
		t.Errorf("ready relays = %v, want 3", got)
	}
}

// Feature: controller domain metrics
// Scenario: the collectors register with the controller-runtime registry
//
//	Given the iris collectors
//	When  the controller-runtime registry is gathered
//	Then  the iris_postfix_config_renders_total metric is present so it serves
//	      on the manager's metrics endpoint
func TestCollectorsRegisteredWithControllerRuntime(t *testing.T) {
	PostfixConfigRenders.Reset()
	PostfixConfigRenders.WithLabelValues("written").Inc()
	count, err := testutil.GatherAndCount(ctrlmetrics.Registry, "iris_postfix_config_renders_total")
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if count == 0 {
		t.Error("iris_postfix_config_renders_total not registered with the controller-runtime registry")
	}
}
