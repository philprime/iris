/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/philprime/iris/api/v1alpha1"
)

// postfixMapsKey is the aggregate ConfigMap the ConfigReconciler writes in the
// envtest suite. It lives in the default namespace, which envtest provides.
var postfixMapsKey = types.NamespacedName{Namespace: "default", Name: "iris-postfix-maps"}

// resetCluster removes every Relay (clearing finalizers first so deletion
// completes) and the aggregate ConfigMap, so each envtest test starts clean
// even though the ConfigReconciler lists relays cluster-wide.
func resetCluster(t *testing.T, c client.Client) {
	t.Helper()
	ctx := context.Background()
	var relays v1alpha1.RelayList
	if err := c.List(ctx, &relays); err != nil {
		t.Fatalf("list relays for reset: %v", err)
	}
	for i := range relays.Items {
		rel := &relays.Items[i]
		if len(rel.Finalizers) > 0 {
			rel.Finalizers = nil
			if err := c.Update(ctx, rel); err != nil && !apierrors.IsNotFound(err) {
				t.Fatalf("clear finalizers on %s: %v", rel.Name, err)
			}
		}
		if err := c.Delete(ctx, rel); err != nil && !apierrors.IsNotFound(err) {
			t.Fatalf("delete relay %s: %v", rel.Name, err)
		}
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: postfixMapsKey.Name, Namespace: postfixMapsKey.Namespace}}
	if err := c.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("delete postfix configmap: %v", err)
	}
}

// createRelay builds and creates a Relay claiming the given routes. The API
// server assigns the creation timestamp, so conflict ordering between relays
// created in the same second falls back to name order.
func createRelay(t *testing.T, c client.Client, name string, routes ...v1alpha1.Route) *v1alpha1.Relay {
	t.Helper()
	rel := relayClaiming(name, time.Time{}, routes...)
	rel.ResourceVersion = ""
	rel.CreationTimestamp = metav1.Time{}
	if err := c.Create(context.Background(), rel); err != nil {
		t.Fatalf("create relay %s: %v", name, err)
	}
	return rel
}

func newEnvtestRelayReconciler(c client.Client) *RelayReconciler {
	return &RelayReconciler{Client: c, Scheme: c.Scheme(), RelayImage: "ghcr.io/philprime/iris-relay:test"}
}

func newEnvtestConfigReconciler(c client.Client) *ConfigReconciler {
	return &ConfigReconciler{Client: c, Scheme: c.Scheme(), PostfixConfigMap: postfixMapsKey}
}

// Feature: per-relay child reconciliation (against a real API server)
// Scenario: creating a Relay makes its Deployment, Service, and ConfigMap appear
func TestEnvtestRelayCreatesChildren(t *testing.T) {
	c := envtestClient(t)
	resetCluster(t, c)
	ctx := context.Background()

	rel := createRelay(t, c, "alpha", v1alpha1.Route{Domain: "invite.example.com"})
	r := newEnvtestRelayReconciler(c)
	if _, err := r.Reconcile(ctx, requestFor(rel)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	childKey := types.NamespacedName{Namespace: "default", Name: "relay-alpha"}
	var dep appsv1.Deployment
	if err := c.Get(ctx, childKey, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	var svc corev1.Service
	if err := c.Get(ctx, childKey, &svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "relay-alpha-config"}, &cm); err != nil {
		t.Fatalf("get config configmap: %v", err)
	}

	if !ownedBy(dep.OwnerReferences, "alpha") {
		t.Errorf("deployment not controller-owned by alpha: %+v", dep.OwnerReferences)
	}

	var got v1alpha1.Relay
	if err := c.Get(ctx, client.ObjectKeyFromObject(rel), &got); err != nil {
		t.Fatalf("get relay: %v", err)
	}
	if got.Status.ServiceRef == nil || got.Status.ServiceRef.Name != "relay-alpha" {
		t.Errorf("status.serviceRef = %+v, want relay-alpha", got.Status.ServiceRef)
	}
}

// Feature: cluster-wide route conflict resolution (against a real API server)
// Scenario: a second relay claiming an owned route is marked Conflict and excluded
func TestEnvtestConflictMarksLoserAndExcludes(t *testing.T) {
	c := envtestClient(t)
	resetCluster(t, c)
	ctx := context.Background()

	// alpha sorts before beta, so on a creation-timestamp tie alpha wins the route.
	createRelay(t, c, "alpha", v1alpha1.Route{Domain: "invite.example.com"})
	beta := createRelay(t, c, "beta", v1alpha1.Route{Domain: "invite.example.com"})

	r := newEnvtestConfigReconciler(c)
	if _, err := r.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var gotBeta v1alpha1.Relay
	if err := c.Get(ctx, client.ObjectKeyFromObject(beta), &gotBeta); err != nil {
		t.Fatalf("get beta: %v", err)
	}
	if !apimeta.IsStatusConditionTrue(gotBeta.Status.Conditions, conditionConflict) {
		t.Errorf("beta Conflict condition not True: %+v", gotBeta.Status.Conditions)
	}
	if len(gotBeta.Status.ClaimedRoutes) != 0 {
		t.Errorf("beta claimed routes = %v, want none", gotBeta.Status.ClaimedRoutes)
	}

	var cm corev1.ConfigMap
	if err := c.Get(ctx, postfixMapsKey, &cm); err != nil {
		t.Fatalf("get postfix configmap: %v", err)
	}
	transport := cm.Data[keyTransport]
	if !strings.Contains(transport, "relay-alpha") {
		t.Errorf("transport missing alpha route: %q", transport)
	}
	if strings.Contains(transport, "relay-beta") {
		t.Errorf("transport should exclude the conflicting beta route: %q", transport)
	}
}

// Feature: finalizer-gated route release (against a real API server)
// Scenario: deleting a Relay releases its route once the finalizer is removed
func TestEnvtestDeletionReleasesRoute(t *testing.T) {
	c := envtestClient(t)
	resetCluster(t, c)
	ctx := context.Background()

	rel := createRelay(t, c, "alpha", v1alpha1.Route{Domain: "invite.example.com"})
	relayR := newEnvtestRelayReconciler(c)
	configR := newEnvtestConfigReconciler(c)

	// First reconcile adds the finalizer and the children, then programs the route.
	if _, err := relayR.Reconcile(ctx, requestFor(rel)); err != nil {
		t.Fatalf("reconcile relay: %v", err)
	}
	if _, err := configR.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatalf("reconcile config: %v", err)
	}

	var withFinalizer v1alpha1.Relay
	if err := c.Get(ctx, client.ObjectKeyFromObject(rel), &withFinalizer); err != nil {
		t.Fatalf("get relay: %v", err)
	}
	if len(withFinalizer.Finalizers) == 0 {
		t.Fatalf("expected finalizer on relay, got none")
	}

	// Deleting only sets the deletion timestamp while the finalizer is held.
	if err := c.Delete(ctx, &withFinalizer); err != nil {
		t.Fatalf("delete relay: %v", err)
	}

	// Reconciling the deletion releases the finalizer so the object is removed.
	if _, err := relayR.Reconcile(ctx, requestFor(rel)); err != nil {
		t.Fatalf("reconcile deletion: %v", err)
	}
	var gone v1alpha1.Relay
	if err := c.Get(ctx, client.ObjectKeyFromObject(rel), &gone); !apierrors.IsNotFound(err) {
		t.Fatalf("relay still present after finalizer release: err=%v", err)
	}

	// The aggregate maps no longer route the released domain.
	if _, err := configR.Reconcile(ctx, reconcile.Request{}); err != nil {
		t.Fatalf("re-render config: %v", err)
	}
	var cm corev1.ConfigMap
	if err := c.Get(ctx, postfixMapsKey, &cm); err != nil {
		t.Fatalf("get postfix configmap: %v", err)
	}
	if strings.Contains(cm.Data[keyTransport], "relay-alpha") {
		t.Errorf("transport still routes the deleted relay: %q", cm.Data[keyTransport])
	}
}
