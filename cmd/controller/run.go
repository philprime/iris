/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/philprime/iris/api/v1alpha1"
	"github.com/philprime/iris/internal/config"
	"github.com/philprime/iris/internal/controller"
	"github.com/philprime/iris/internal/observability"
	"github.com/philprime/iris/internal/postfix"
	iriswebhook "github.com/philprime/iris/internal/webhook"
)

func run(ctx context.Context) error {
	var cfg config.Controller
	if err := config.Load(&cfg); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Sentry.Release == "" {
		cfg.Sentry.Release = sentryReleaseID()
	}

	terminal := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger, flush := observability.Setup(ctx, cfg.Sentry, terminal)
	defer flush()
	ctrl.SetLogger(observability.LogrFromSlog(logger))

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register client-go scheme: %w", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register iris scheme: %w", err)
	}

	webhookHost, webhookPort, err := splitHostPort(cfg.WebhookAddr)
	if err != nil {
		return fmt.Errorf("parse webhook address %q: %w", cfg.WebhookAddr, err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: cfg.MetricsAddr},
		HealthProbeBindAddress: cfg.HealthAddr,
		LeaderElection:         cfg.LeaderElect,
		LeaderElectionID:       "iris-controller.philprime.dev",
		LeaseDuration:          ptr(15 * time.Second),
		RenewDeadline:          ptr(10 * time.Second),
		WebhookServer:          webhook.NewServer(webhook.Options{Host: webhookHost, Port: webhookPort}),
	})
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	configReconciler := &controller.ConfigReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		PostfixConfigMap: types.NamespacedName{Namespace: cfg.Namespace, Name: cfg.PostfixConfigMap},
		RenderOptions:    postfix.Options{ClusterDomain: cfg.ClusterDomain},
	}
	if err := configReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("set up config reconciler: %w", err)
	}

	relayReconciler := &controller.RelayReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		RelayImage: cfg.RelayImage,
	}
	if err := relayReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("set up relay reconciler: %w", err)
	}

	if cfg.EnableWebhook {
		if err := iriswebhook.SetupRelayWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("set up relay webhook: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	readyz := healthz.Ping
	if cfg.EnableWebhook {
		readyz = mgr.GetWebhookServer().StartedChecker()
	}
	if err := mgr.AddReadyzCheck("ready", readyz); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	logger.InfoContext(ctx, "starting iris controller",
		slog.String("version", version),
		slog.String("commit", commit),
		slog.String("buildDate", date),
		slog.Bool("leaderElection", cfg.LeaderElect),
		slog.Bool("webhook", cfg.EnableWebhook))
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("run manager: %w", err)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

// sentryReleaseID resolves the Sentry release: the ldflags-injected value when
// set, otherwise derived from the build version and commit.
func sentryReleaseID() string {
	if sentryRelease != "" {
		return sentryRelease
	}
	return observability.ReleaseID(version, commit)
}

// splitHostPort parses a "host:port" bind address into its host and integer
// port. An empty host (for example ":9443") binds all interfaces.
func splitHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, port, nil
}
