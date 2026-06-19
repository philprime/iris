/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/philprime/iris/api/v1alpha1"
)

// Feature: relay data-plane config rendering
// Scenario: a Relay's routing intent renders to versioned YAML
//
//	Given a Relay with routes, filters, idempotency, and destinations
//	When  the config is rendered
//	Then  it round-trips to YAML carrying a version and the spec subset, but not
//	      the deployment shape (which is a control-plane concern)
func TestRenderConfigRoundTrips(t *testing.T) {
	relay := &v1alpha1.Relay{
		Spec: v1alpha1.RelaySpec{
			Routes:      []v1alpha1.Route{{Domain: "invite.example.com"}},
			Idempotency: v1alpha1.IdempotencySHA256,
			Filters:     &v1alpha1.Filters{MaxMessageBytes: 1024},
			Destinations: []v1alpha1.Destination{
				{Name: "webhook", HTTP: &v1alpha1.HTTPDestination{URL: "https://example.test/in"}},
			},
			Deployment: &v1alpha1.DeploymentSpec{},
		},
	}

	out, err := RenderConfig(relay)
	if err != nil {
		t.Fatalf("render config: %v", err)
	}

	var got Config
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal rendered config: %v", err)
	}

	if got.Version != ConfigVersion {
		t.Errorf("version = %q, want %q", got.Version, ConfigVersion)
	}
	if got.Idempotency != v1alpha1.IdempotencySHA256 {
		t.Errorf("idempotency = %q, want sha256", got.Idempotency)
	}
	if len(got.Routes) != 1 || got.Routes[0].Domain != "invite.example.com" {
		t.Errorf("routes = %v, want [invite.example.com]", got.Routes)
	}
	if got.Filters == nil || got.Filters.MaxMessageBytes != 1024 {
		t.Errorf("filters = %v, want maxMessageBytes 1024", got.Filters)
	}
	if len(got.Destinations) != 1 || got.Destinations[0].Name != "webhook" {
		t.Errorf("destinations = %v, want [webhook]", got.Destinations)
	}
}

// Feature: relay data-plane config rendering
// Scenario: rendering is byte-stable for an unchanged Relay
//
//	Given the same Relay rendered twice
//	When  the outputs are compared
//	Then  they are identical so the controller can skip no-op ConfigMap writes
func TestRenderConfigIsStable(t *testing.T) {
	relay := &v1alpha1.Relay{
		Spec: v1alpha1.RelaySpec{
			Routes:       []v1alpha1.Route{{Address: "a@example.com"}},
			Destinations: []v1alpha1.Destination{{Name: "d", SMTP: &v1alpha1.SMTPDestination{Host: "h", Port: 25}}},
		},
	}
	first, err := RenderConfig(relay)
	if err != nil {
		t.Fatalf("render first: %v", err)
	}
	second, err := RenderConfig(relay)
	if err != nil {
		t.Fatalf("render second: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("render not stable:\n%s\n---\n%s", first, second)
	}
}
