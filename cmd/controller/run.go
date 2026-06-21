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
	"os"

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
		cfg.Sentry.Release = observability.ResolveRelease(sentryRelease, version, commit)
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

	webhookHost, webhookPort, err := config.SplitHostPort(cfg.WebhookAddr)
	if err != nil {
		return fmt.Errorf("parse webhook address %q: %w", cfg.WebhookAddr, err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: cfg.MetricsAddr},
		HealthProbeBindAddress: cfg.HealthAddr,
		LeaderElection:         cfg.LeaderElect,
		LeaderElectionID:       "iris-controller.philprime.dev",
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
