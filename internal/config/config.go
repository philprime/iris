/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package config loads each binary's settings from the environment and
// validates them at startup. Variables are prefixed IRIS_<COMPONENT>_<SETTING>
// and validated with go-playground/validator struct tags, so a misconfigured
// binary fails fast instead of starting in a bad state.
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

// Sentry holds the error-reporting settings shared by every binary. The
// defaults keep local and air-gapped installs running clean (disabled, tracing
// off).
type Sentry struct {
	Enabled          bool    `env:"IRIS_SENTRY_ENABLED" envDefault:"false"`
	DSN              string  `env:"IRIS_SENTRY_DSN"`
	Environment      string  `env:"IRIS_SENTRY_ENVIRONMENT" envDefault:"local"`
	Release          string  `env:"IRIS_SENTRY_RELEASE"`
	Debug            bool    `env:"IRIS_SENTRY_DEBUG" envDefault:"false"`
	AttachStacktrace bool    `env:"IRIS_SENTRY_ATTACH_STACKTRACE" envDefault:"true"`
	SampleRate       float64 `env:"IRIS_SENTRY_SAMPLE_RATE" envDefault:"1.0" validate:"gte=0,lte=1"`
	EnableTracing    bool    `env:"IRIS_SENTRY_ENABLE_TRACING" envDefault:"false"`
	TracesSampleRate float64 `env:"IRIS_SENTRY_TRACES_SAMPLE_RATE" envDefault:"0.0" validate:"gte=0,lte=1"`
}

// Controller holds the controller manager's settings.
type Controller struct {
	MetricsAddr      string `env:"IRIS_CONTROLLER_METRICS_ADDR" envDefault:":8080" validate:"required"`
	HealthAddr       string `env:"IRIS_CONTROLLER_HEALTH_ADDR" envDefault:":8081" validate:"required"`
	WebhookAddr      string `env:"IRIS_CONTROLLER_WEBHOOK_ADDR" envDefault:":9443" validate:"required"`
	LeaderElect      bool   `env:"IRIS_CONTROLLER_LEADER_ELECT" envDefault:"true"`
	EnableWebhook    bool   `env:"IRIS_CONTROLLER_ENABLE_WEBHOOK" envDefault:"true"`
	Namespace        string `env:"IRIS_CONTROLLER_NAMESPACE" envDefault:"iris-system" validate:"required"`
	PostfixConfigMap string `env:"IRIS_CONTROLLER_POSTFIX_CONFIGMAP" envDefault:"iris-postfix-maps" validate:"required"`
	RelayImage       string `env:"IRIS_CONTROLLER_RELAY_IMAGE" envDefault:"ghcr.io/philprime/iris-relay:latest" validate:"required"`
	ClusterDomain    string `env:"IRIS_CONTROLLER_CLUSTER_DOMAIN" envDefault:"cluster.local" validate:"required"`
	Sentry           Sentry
}

// Relay holds the relay data-plane server's settings.
type Relay struct {
	ConfigPath string `env:"IRIS_RELAY_CONFIG" envDefault:"/etc/iris/relay/config.yaml" validate:"required"`
	MountDir   string `env:"IRIS_RELAY_MOUNT_DIR" envDefault:"/etc/iris/relay" validate:"required"`
	SMTPAddr   string `env:"IRIS_RELAY_SMTP_ADDR" envDefault:":25" validate:"required"`
	AdminAddr  string `env:"IRIS_RELAY_ADMIN_ADDR" envDefault:":8080" validate:"required"`
	Sentry     Sentry
}

// Reloader holds the Postfix reloader's settings.
type Reloader struct {
	WatchPath string `env:"IRIS_RELOADER_WATCH_PATH" envDefault:"/etc/postfix/maps" validate:"required"`
	AdminAddr string `env:"IRIS_RELOADER_ADMIN_ADDR" envDefault:":8080" validate:"required"`
	Sentry    Sentry
}

// Load parses the environment into cfg (which must be a pointer to a config
// struct) and validates it. It returns an error if either step fails.
func Load(cfg any) error {
	if err := env.Parse(cfg); err != nil {
		return fmt.Errorf("parse environment: %w", err)
	}
	if err := validator.New().Struct(cfg); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	return nil
}
