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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/philprime/iris/api/v1alpha1"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add appsv1 to scheme: %v", err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add v1alpha1 to scheme: %v", err)
	}
	return scheme
}

func relayClaiming(name string, created time.Time, routes ...v1alpha1.Route) *v1alpha1.Relay {
	return &v1alpha1.Relay{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: v1alpha1.RelaySpec{
			Routes: routes,
			Destinations: []v1alpha1.Destination{
				{Name: "webhook", HTTP: &v1alpha1.HTTPDestination{URL: "https://example.test/in"}},
			},
		},
	}
}

func newConfigReconciler(scheme *runtime.Scheme, objs ...client.Object) (*ConfigReconciler, client.Client) {
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.Relay{}).
		Build()
	return &ConfigReconciler{
		Client:           c,
		Scheme:           scheme,
		PostfixConfigMap: types.NamespacedName{Namespace: "iris-system", Name: "iris-postfix-maps"},
	}, c
}

// Feature: Postfix map aggregation
// Scenario: a single relay's routes compile into the Postfix maps ConfigMap
//
//	Given one Relay claiming a domain
//	When  the ConfigReconciler reconciles
//	Then  the Postfix maps ConfigMap holds the rendered transport/domains/recipients
func TestConfigReconcileWritesPostfixConfigMap(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	relay := relayClaiming("alpha", base, v1alpha1.Route{Domain: "invite.example.com"})
	r, c := newConfigReconciler(scheme, relay)

	if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), r.PostfixConfigMap, &cm); err != nil {
		t.Fatalf("get postfix configmap: %v", err)
	}
	if !strings.Contains(cm.Data["transport"], "invite.example.com") {
		t.Errorf("transport map missing route: %q", cm.Data["transport"])
	}
	if !strings.Contains(cm.Data["transport"], "relay-alpha.default.svc") {
		t.Errorf("transport map missing service target: %q", cm.Data["transport"])
	}
	if got := strings.TrimSpace(cm.Data["relay_domains"]); got != "invite.example.com" {
		t.Errorf("relay_domains = %q, want invite.example.com", got)
	}
}

// Feature: Postfix map aggregation
// Scenario: conflicting relays resolve first-writer-wins and report status
//
//	Given two Relays claiming the same domain, created at different times
//	When  the ConfigReconciler reconciles
//	Then  the earlier relay is Programmed and the later one is Conflict=True
func TestConfigReconcileResolvesConflicts(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	winner := relayClaiming("winner", base, v1alpha1.Route{Domain: "invite.example.com"})
	loser := relayClaiming("loser", base.Add(time.Hour), v1alpha1.Route{Domain: "invite.example.com"})
	r, c := newConfigReconciler(scheme, winner, loser)

	if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := func(name string) *v1alpha1.Relay {
		var relay v1alpha1.Relay
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: name}, &relay); err != nil {
			t.Fatalf("get relay %s: %v", name, err)
		}
		return &relay
	}

	w := got("winner")
	if cond := apimeta.FindStatusCondition(w.Status.Conditions, conditionProgrammed); cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("winner Programmed = %v, want True", cond)
	}
	if apimeta.IsStatusConditionTrue(w.Status.Conditions, conditionConflict) {
		t.Errorf("winner unexpectedly Conflict=True")
	}
	if len(w.Status.ClaimedRoutes) != 1 || w.Status.ClaimedRoutes[0] != "@invite.example.com" {
		t.Errorf("winner claimedRoutes = %v, want [@invite.example.com]", w.Status.ClaimedRoutes)
	}

	l := got("loser")
	if !apimeta.IsStatusConditionTrue(l.Status.Conditions, conditionConflict) {
		t.Errorf("loser Conflict = %v, want True", l.Status.Conditions)
	}
	if apimeta.IsStatusConditionTrue(l.Status.Conditions, conditionProgrammed) {
		t.Errorf("loser unexpectedly Programmed=True")
	}
	if len(l.Status.ClaimedRoutes) != 0 {
		t.Errorf("loser claimedRoutes = %v, want empty", l.Status.ClaimedRoutes)
	}
}
