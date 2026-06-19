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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/philprime/iris/api/v1alpha1"
	"github.com/philprime/iris/internal/controller"
	"github.com/philprime/iris/internal/postfix"
	iriswebhook "github.com/philprime/iris/internal/webhook"
)

// config holds the controller's runtime settings, sourced from the environment.
type config struct {
	metricsAddr       string
	healthAddr        string
	webhookPort       int
	enableLeaderElect bool
	postfixConfigMap  types.NamespacedName
	relayImage        string
	clusterDomain     string
	enableWebhook     bool
}

func loadConfig() config {
	return config{
		metricsAddr:       env("IRIS_CONTROLLER_METRICS_ADDR", ":8080"),
		healthAddr:        env("IRIS_CONTROLLER_HEALTH_ADDR", ":8081"),
		webhookPort:       envInt("IRIS_CONTROLLER_WEBHOOK_PORT", 9443),
		enableLeaderElect: envBool("IRIS_CONTROLLER_LEADER_ELECT", true),
		postfixConfigMap: types.NamespacedName{
			Namespace: env("IRIS_CONTROLLER_NAMESPACE", "iris-system"),
			Name:      env("IRIS_CONTROLLER_POSTFIX_CONFIGMAP", "iris-postfix-maps"),
		},
		relayImage:    env("IRIS_CONTROLLER_RELAY_IMAGE", "ghcr.io/philprime/iris-relay:latest"),
		clusterDomain: env("IRIS_CONTROLLER_CLUSTER_DOMAIN", "cluster.local"),
		enableWebhook: envBool("IRIS_CONTROLLER_ENABLE_WEBHOOK", true),
	}
}

func run(ctx context.Context) error {
	cfg := loadConfig()

	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)
	ctrl.SetLogger(logr.FromSlogHandler(handler))

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register client-go scheme: %w", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("register iris scheme: %w", err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: cfg.metricsAddr},
		HealthProbeBindAddress: cfg.healthAddr,
		LeaderElection:         cfg.enableLeaderElect,
		LeaderElectionID:       "iris-controller.philprime.dev",
		LeaseDuration:          ptr(15 * time.Second),
		RenewDeadline:          ptr(10 * time.Second),
		WebhookServer:          webhook.NewServer(webhook.Options{Port: cfg.webhookPort}),
	})
	if err != nil {
		return fmt.Errorf("create manager: %w", err)
	}

	configReconciler := &controller.ConfigReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		PostfixConfigMap: cfg.postfixConfigMap,
		RenderOptions:    postfix.Options{ClusterDomain: cfg.clusterDomain},
	}
	if err := configReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("set up config reconciler: %w", err)
	}

	relayReconciler := &controller.RelayReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		RelayImage: cfg.relayImage,
	}
	if err := relayReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("set up relay reconciler: %w", err)
	}

	if cfg.enableWebhook {
		if err := iriswebhook.SetupRelayWebhookWithManager(mgr); err != nil {
			return fmt.Errorf("set up relay webhook: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		return fmt.Errorf("add healthz check: %w", err)
	}
	readyz := healthz.Ping
	if cfg.enableWebhook {
		readyz = mgr.GetWebhookServer().StartedChecker()
	}
	if err := mgr.AddReadyzCheck("ready", readyz); err != nil {
		return fmt.Errorf("add readyz check: %w", err)
	}

	logger.InfoContext(ctx, "starting iris controller",
		slog.Bool("leaderElection", cfg.enableLeaderElect),
		slog.Bool("webhook", cfg.enableWebhook))
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("run manager: %w", err)
	}
	return nil
}

func ptr[T any](v T) *T { return &v }

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch env(key, "") {
	case "":
		return fallback
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
