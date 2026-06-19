/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package relay holds the stateless data-plane logic and the config contract
// shared between the controller (which renders it) and the relay binary (which
// consumes it).
package relay

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/philprime/iris/api/v1alpha1"
)

// ConfigVersion is the schema version of the rendered relay config. It lets the
// relay binary reject a config it cannot understand.
const ConfigVersion = "v1"

// ConfigFileName is the key under which the rendered config is stored in the
// per-relay ConfigMap and the path it mounts to inside the relay pod.
const ConfigFileName = "config.yaml"

// Config is the data-plane view of a Relay: the routing, filtering, and
// delivery intent with the control-plane-only deployment shape dropped. It
// reuses the api/v1alpha1 structs so the spec stays the single source of truth.
type Config struct {
	// Version is the schema version of this document.
	Version string `json:"version"`
	// Routes are the recipients the relay accepts.
	Routes []v1alpha1.Route `json:"routes"`
	// Filters optionally rejects inbound mail before delivery.
	Filters *v1alpha1.Filters `json:"filters,omitempty"`
	// Idempotency selects the key sent to every destination.
	Idempotency v1alpha1.IdempotencyMode `json:"idempotency,omitempty"`
	// Destinations receive every accepted message.
	Destinations []v1alpha1.Destination `json:"destinations"`
}

// ConfigFromRelay projects a Relay spec into the data-plane Config.
func ConfigFromRelay(relay *v1alpha1.Relay) Config {
	return Config{
		Version:      ConfigVersion,
		Routes:       relay.Spec.Routes,
		Filters:      relay.Spec.Filters,
		Idempotency:  relay.Spec.Idempotency,
		Destinations: relay.Spec.Destinations,
	}
}

// RenderConfig renders a Relay's data-plane Config as YAML. The output is
// byte-stable for an unchanged Relay so the controller can skip no-op writes.
func RenderConfig(relay *v1alpha1.Relay) ([]byte, error) {
	out, err := yaml.Marshal(ConfigFromRelay(relay))
	if err != nil {
		return nil, fmt.Errorf("marshal relay config: %w", err)
	}
	return out, nil
}
