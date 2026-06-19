/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/philprime/iris/api/v1alpha1"
)

// Feature: target resolution
// Scenario: HTTP destination secrets and transforms are read from mounts
//
//	Given a config with an HTTP destination referencing a secret and a transform
//	When  targets are built against the mount directory
//	Then  the auth token and jsonnet program are resolved from the mounted files
func TestBuildTargetsResolvesHTTPMounts(t *testing.T) {
	dir := t.TempDir()
	writeMount(t, filepath.Join(dir, "secrets", "webhook", "token"), "secret-token")
	writeMount(t, filepath.Join(dir, "transforms", "mapping", "map.jsonnet"), "{ x: 1 }")

	cfg := Config{
		Destinations: []v1alpha1.Destination{{
			Name: "webhook",
			HTTP: &v1alpha1.HTTPDestination{
				URL:           "https://service.internal/in",
				PayloadFormat: v1alpha1.PayloadFormatJSON,
				AuthSecretRef: &v1alpha1.SecretKeyRef{Name: "webhook", Key: "token"},
				Transform:     &v1alpha1.Transform{JsonnetConfigMapRef: v1alpha1.ConfigMapKeyRef{Name: "mapping", Key: "map.jsonnet"}},
			},
		}},
	}

	targets, err := BuildTargets(cfg, dir, nil)
	if err != nil {
		t.Fatalf("build targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	target := targets[0]
	if !target.Required {
		t.Error("destination should default to required")
	}
	if target.HTTP == nil || target.HTTP.AuthToken != "secret-token" {
		t.Errorf("auth token not resolved: %+v", target.HTTP)
	}
	if target.HTTP.Jsonnet != "{ x: 1 }" {
		t.Errorf("jsonnet = %q, want { x: 1 }", target.HTTP.Jsonnet)
	}
}

// Feature: target resolution
// Scenario: an SMTP destination becomes an SMTP target
func TestBuildTargetsResolvesSMTP(t *testing.T) {
	notRequired := false
	cfg := Config{
		Destinations: []v1alpha1.Destination{{
			Name:     "archive",
			Required: &notRequired,
			SMTP:     &v1alpha1.SMTPDestination{Host: "archive.internal", Port: 1025},
		}},
	}

	targets, err := BuildTargets(cfg, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("build targets: %v", err)
	}
	if targets[0].Required {
		t.Error("destination should respect required=false")
	}
	if targets[0].SMTP == nil || targets[0].SMTP.Host != "archive.internal" || targets[0].SMTP.Port != 1025 {
		t.Errorf("smtp target not resolved: %+v", targets[0].SMTP)
	}
}

func writeMount(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}
