/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package config

import "testing"

// Feature: controller configuration
// Scenario: defaults apply when the environment is empty
//
//	Given no IRIS_CONTROLLER_* variables set
//	When  the controller config is loaded
//	Then  the documented defaults are used
func TestControllerDefaults(t *testing.T) {
	var c Controller
	if err := Load(&c); err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.MetricsAddr != ":8080" {
		t.Errorf("MetricsAddr = %q, want :8080", c.MetricsAddr)
	}
	if c.WebhookAddr != ":9443" {
		t.Errorf("WebhookAddr = %q, want :9443", c.WebhookAddr)
	}
	if !c.LeaderElect {
		t.Error("LeaderElect = false, want true by default")
	}
	if c.Sentry.Enabled {
		t.Error("Sentry.Enabled = true, want false by default")
	}
	if c.Sentry.Environment != "local" {
		t.Errorf("Sentry.Environment = %q, want local", c.Sentry.Environment)
	}
}

// Feature: controller configuration
// Scenario: environment variables override the defaults
func TestControllerFromEnv(t *testing.T) {
	t.Setenv("IRIS_CONTROLLER_METRICS_ADDR", ":9090")
	t.Setenv("IRIS_CONTROLLER_LEADER_ELECT", "false")
	t.Setenv("IRIS_CONTROLLER_RELAY_IMAGE", "ghcr.io/philprime/iris-relay:v1")

	var c Controller
	if err := Load(&c); err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.MetricsAddr != ":9090" {
		t.Errorf("MetricsAddr = %q, want :9090", c.MetricsAddr)
	}
	if c.LeaderElect {
		t.Error("LeaderElect = true, want false from env")
	}
	if c.RelayImage != "ghcr.io/philprime/iris-relay:v1" {
		t.Errorf("RelayImage = %q, want the env override", c.RelayImage)
	}
}

// Feature: controller configuration
// Scenario: invalid values are rejected at load time
//
//	Given a Sentry sample rate above 1.0
//	When  the config is loaded
//	Then  validation fails so the binary refuses to start misconfigured
func TestControllerValidationRejectsBadSampleRate(t *testing.T) {
	t.Setenv("IRIS_SENTRY_SAMPLE_RATE", "5")
	var c Controller
	if err := Load(&c); err == nil {
		t.Fatal("expected validation error for sample rate above 1.0")
	}
}
