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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/philprime/iris/api/v1alpha1"
	"github.com/philprime/iris/internal/relay"
)

func newRelayReconciler(scheme *runtime.Scheme, objs ...client.Object) (*RelayReconciler, client.Client) {
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.Relay{}).
		Build()
	return &RelayReconciler{
		Client:     c,
		Scheme:     scheme,
		RelayImage: "ghcr.io/philprime/iris-relay:test",
	}, c
}

func requestFor(relay *v1alpha1.Relay) reconcile.Request {
	return reconcile.Request{NamespacedName: client.ObjectKeyFromObject(relay)}
}

func ownedBy(refs []metav1.OwnerReference, name string) bool {
	for _, ref := range refs {
		if ref.Kind == "Relay" && ref.Name == name && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

// Feature: per-relay child reconciliation
// Scenario: a Relay reconciles into a Deployment, Service, and config ConfigMap
//
//	Given a Relay
//	When  the RelayReconciler reconciles it
//	Then  the three children exist, are controller-owned, and status.serviceRef is set
func TestRelayReconcileCreatesChildren(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rel := relayClaiming("alpha", base, v1alpha1.Route{Domain: "invite.example.com"})
	r, c := newRelayReconciler(scheme, rel)

	if _, err := r.Reconcile(context.Background(), requestFor(rel)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	ctx := context.Background()
	childKey := types.NamespacedName{Namespace: "default", Name: "relay-alpha"}

	var svc corev1.Service
	if err := c.Get(ctx, childKey, &svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if !ownedBy(svc.OwnerReferences, "alpha") {
		t.Errorf("service not controller-owned by relay alpha: %v", svc.OwnerReferences)
	}

	var dep appsv1.Deployment
	if err := c.Get(ctx, childKey, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if !ownedBy(dep.OwnerReferences, "alpha") {
		t.Errorf("deployment not controller-owned by relay alpha: %v", dep.OwnerReferences)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "ghcr.io/philprime/iris-relay:test" {
		t.Errorf("deployment image = %q, want the configured relay image", dep.Spec.Template.Spec.Containers[0].Image)
	}

	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "relay-alpha-config"}, &cm); err != nil {
		t.Fatalf("get config configmap: %v", err)
	}
	if !strings.Contains(cm.Data[relay.ConfigFileName], "invite.example.com") {
		t.Errorf("relay config missing route: %q", cm.Data[relay.ConfigFileName])
	}

	var got v1alpha1.Relay
	if err := c.Get(ctx, client.ObjectKeyFromObject(rel), &got); err != nil {
		t.Fatalf("get relay: %v", err)
	}
	if got.Status.ServiceRef == nil || got.Status.ServiceRef.Name != "relay-alpha" {
		t.Errorf("status.serviceRef = %v, want relay-alpha", got.Status.ServiceRef)
	}
}

// Feature: per-relay child reconciliation
// Scenario: destination secrets and transforms are mounted into the relay pod
//
//	Given a Relay whose destination references an auth Secret and a Jsonnet ConfigMap
//	When  the RelayReconciler reconciles it
//	Then  both are mounted at the paths the relay reads, with the mount-dir env set
func TestRelayReconcileMountsSecretsAndTransforms(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rel := &v1alpha1.Relay{
		ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "default", CreationTimestamp: metav1.NewTime(base)},
		Spec: v1alpha1.RelaySpec{
			Routes: []v1alpha1.Route{{Domain: "invite.example.com"}},
			Destinations: []v1alpha1.Destination{{
				Name: "webhook",
				HTTP: &v1alpha1.HTTPDestination{
					URL:           "https://service.internal/in",
					AuthSecretRef: &v1alpha1.SecretKeyRef{Name: "webhook-secret", Key: "token"},
					Transform:     &v1alpha1.Transform{JsonnetConfigMapRef: v1alpha1.ConfigMapKeyRef{Name: "map-cm", Key: "map.jsonnet"}},
				},
			}},
		},
	}
	r, c := newRelayReconciler(scheme, rel)

	if _, err := r.Reconcile(context.Background(), requestFor(rel)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var dep appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "relay-alpha"}, &dep); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	pod := dep.Spec.Template.Spec

	secretVol := volumeForSecret(pod.Volumes, "webhook-secret")
	if secretVol == "" {
		t.Fatalf("no volume sourced from secret webhook-secret: %+v", pod.Volumes)
	}
	if path := mountPath(pod.Containers[0].VolumeMounts, secretVol); path != "/etc/iris/relay/secrets/webhook-secret" {
		t.Errorf("secret mount path = %q, want /etc/iris/relay/secrets/webhook-secret", path)
	}

	transformVol := volumeForConfigMap(pod.Volumes, "map-cm")
	if transformVol == "" {
		t.Fatalf("no volume sourced from configmap map-cm: %+v", pod.Volumes)
	}
	if path := mountPath(pod.Containers[0].VolumeMounts, transformVol); path != "/etc/iris/relay/transforms/map-cm" {
		t.Errorf("transform mount path = %q, want /etc/iris/relay/transforms/map-cm", path)
	}

	if env := envValue(pod.Containers[0].Env, "IRIS_RELAY_MOUNT_DIR"); env != "/etc/iris/relay" {
		t.Errorf("IRIS_RELAY_MOUNT_DIR = %q, want /etc/iris/relay", env)
	}
}

func volumeForSecret(volumes []corev1.Volume, secretName string) string {
	for _, v := range volumes {
		if v.Secret != nil && v.Secret.SecretName == secretName {
			return v.Name
		}
	}
	return ""
}

func volumeForConfigMap(volumes []corev1.Volume, cmName string) string {
	for _, v := range volumes {
		if v.ConfigMap != nil && v.ConfigMap.Name == cmName {
			return v.Name
		}
	}
	return ""
}

func mountPath(mounts []corev1.VolumeMount, volumeName string) string {
	for _, m := range mounts {
		if m.Name == volumeName {
			return m.MountPath
		}
	}
	return ""
}

func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// Feature: per-relay child reconciliation
// Scenario: the reconciler installs a finalizer so routes release before deletion
//
//	Given a Relay without a finalizer
//	When  the RelayReconciler reconciles it
//	Then  the relay carries the Iris finalizer
func TestRelayReconcileAddsFinalizer(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rel := relayClaiming("alpha", base, v1alpha1.Route{Domain: "invite.example.com"})
	r, c := newRelayReconciler(scheme, rel)

	if _, err := r.Reconcile(context.Background(), requestFor(rel)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.Relay
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(rel), &got); err != nil {
		t.Fatalf("get relay: %v", err)
	}
	if !controllerutil.ContainsFinalizer(&got, relayFinalizer) {
		t.Errorf("relay missing finalizer %q: %v", relayFinalizer, got.Finalizers)
	}
}

// Feature: per-relay child reconciliation
// Scenario: deleting a Relay releases its finalizer
//
//	Given a Relay marked for deletion that still holds the Iris finalizer
//	When  the RelayReconciler reconciles it
//	Then  the finalizer is removed so the API server can complete deletion
func TestRelayReconcileDeletionRemovesFinalizer(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rel := relayClaiming("alpha", base, v1alpha1.Route{Domain: "invite.example.com"})
	rel.Finalizers = []string{relayFinalizer}
	deleted := metav1.NewTime(base.Add(time.Hour))
	rel.DeletionTimestamp = &deleted
	r, c := newRelayReconciler(scheme, rel)

	if _, err := r.Reconcile(context.Background(), requestFor(rel)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var got v1alpha1.Relay
	err := c.Get(context.Background(), client.ObjectKeyFromObject(rel), &got)
	if err == nil && controllerutil.ContainsFinalizer(&got, relayFinalizer) {
		t.Errorf("finalizer still present after deletion reconcile: %v", got.Finalizers)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("unexpected error getting relay: %v", err)
	}
}
