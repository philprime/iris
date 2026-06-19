/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package controller holds the Iris control-plane reconcilers.
package controller

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/philprime/iris/api/v1alpha1"
	"github.com/philprime/iris/internal/metrics"
	"github.com/philprime/iris/internal/postfix"
)

// Condition types reported on a Relay's status.
const (
	// conditionReady summarizes the owned conditions.
	conditionReady = "Ready"
	// conditionProgrammed reports that a relay's routes are compiled into the
	// Postfix ingress.
	conditionProgrammed = "Programmed"
	// conditionConflict reports that a relay lost a route to an earlier claimant.
	conditionConflict = "Conflict"
)

// Keys of the rendered Postfix maps within the aggregate ConfigMap.
const (
	keyTransport       = "transport"
	keyRelayDomains    = "relay_domains"
	keyRelayRecipients = "relay_recipient_maps"
)

// ConfigReconciler compiles the aggregate Postfix routing maps from every Relay
// and writes them into a single ConfigMap mounted by the Postfix ingress. It
// owns conflict resolution and the Programmed/Conflict conditions.
type ConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// PostfixConfigMap is the ConfigMap that receives the rendered Postfix maps.
	PostfixConfigMap types.NamespacedName
	// RenderOptions tunes how routes render into Postfix targets.
	RenderOptions postfix.Options
}

// SetupWithManager registers the ConfigReconciler to re-render on any Relay
// change. The request is ignored: every reconcile re-lists and re-renders the
// whole aggregate.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Relay{}).
		Named("config").
		Complete(r)
}

// Reconcile re-renders the Postfix maps whenever any Relay changes, then
// reflects each relay's claim outcome into its status.

//+kubebuilder:rbac:groups=iris.philprime.dev,resources=relays,verbs=get;list;watch
//+kubebuilder:rbac:groups=iris.philprime.dev,resources=relays/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

func (r *ConfigReconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	var relays v1alpha1.RelayList
	if err := r.List(ctx, &relays); err != nil {
		return reconcile.Result{}, fmt.Errorf("list relays: %w", err)
	}

	maps, conflicts, err := postfix.Render(relays.Items, r.RenderOptions)
	if err != nil {
		metrics.PostfixConfigRenders.WithLabelValues("error").Inc()
		return reconcile.Result{}, fmt.Errorf("render postfix maps: %w", err)
	}

	result, resourceVersion, err := r.writeConfigMap(ctx, maps)
	if err != nil {
		metrics.PostfixConfigRenders.WithLabelValues("error").Inc()
		return reconcile.Result{}, err
	}
	metrics.PostfixConfigRenders.WithLabelValues(result).Inc()
	if gen, perr := strconv.ParseFloat(resourceVersion, 64); perr == nil {
		metrics.PostfixConfigGeneration.Set(gen)
	}
	metrics.RouteConflicts.Set(float64(len(conflicts)))

	// Index the lost route keys per relay so each relay's status reflects only
	// the routes it actually claimed.
	lost := map[types.NamespacedName]map[string]struct{}{}
	for _, c := range conflicts {
		if lost[c.Relay] == nil {
			lost[c.Relay] = map[string]struct{}{}
		}
		lost[c.Relay][c.Route] = struct{}{}
	}

	for i := range relays.Items {
		relay := &relays.Items[i]
		nn := types.NamespacedName{Namespace: relay.Namespace, Name: relay.Name}
		if err := r.updateRelayStatus(ctx, relay, lost[nn]); err != nil {
			return reconcile.Result{}, err
		}
	}

	recordRelayPhases(relays.Items, lost)

	return reconcile.Result{}, nil
}

// recordRelayPhases sets the iris_relays gauge to the count of relays in each
// phase (ready, conflict, programming), clearing stale series first.
func recordRelayPhases(relays []v1alpha1.Relay, lost map[types.NamespacedName]map[string]struct{}) {
	metrics.Relays.Reset()
	counts := map[string]int{}
	for i := range relays {
		relay := &relays[i]
		nn := types.NamespacedName{Namespace: relay.Namespace, Name: relay.Name}
		counts[relayPhase(relay, lost[nn])]++
	}
	for phase, n := range counts {
		metrics.Relays.WithLabelValues(phase).Set(float64(n))
	}
}

// relayPhase classifies a relay for the iris_relays gauge: conflict when it
// lost a route, ready when it is programmed and its Deployment is available,
// otherwise programming.
func relayPhase(relay *v1alpha1.Relay, lost map[string]struct{}) string {
	if len(lost) > 0 {
		return "conflict"
	}
	claimed := claimedRoutes(relay, lost)
	if len(claimed) > 0 && apimeta.IsStatusConditionTrue(relay.Status.Conditions, conditionDeploymentAvailable) {
		return "ready"
	}
	return "programming"
}

// writeConfigMap creates or updates the aggregate Postfix maps ConfigMap,
// skipping the write when the rendered data is unchanged. It returns the render
// result ("written" or "nochange") and the ConfigMap's resource version.
func (r *ConfigReconciler) writeConfigMap(ctx context.Context, maps postfix.Maps) (string, string, error) {
	data := map[string]string{
		keyTransport:       maps.Transport,
		keyRelayDomains:    maps.RelayDomains,
		keyRelayRecipients: maps.RelayRecipients,
	}

	var cm corev1.ConfigMap
	err := r.Get(ctx, r.PostfixConfigMap, &cm)
	switch {
	case apierrors.IsNotFound(err):
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.PostfixConfigMap.Name,
				Namespace: r.PostfixConfigMap.Namespace,
			},
			Data: data,
		}
		if err := r.Create(ctx, &cm); err != nil {
			return "", "", fmt.Errorf("create postfix configmap: %w", err)
		}
		return "written", cm.ResourceVersion, nil
	case err != nil:
		return "", "", fmt.Errorf("get postfix configmap: %w", err)
	}

	if mapsEqual(cm.Data, data) {
		return "nochange", cm.ResourceVersion, nil
	}
	original := cm.DeepCopy()
	cm.Data = data
	if err := r.Patch(ctx, &cm, client.MergeFrom(original)); err != nil {
		return "", "", fmt.Errorf("patch postfix configmap: %w", err)
	}
	return "written", cm.ResourceVersion, nil
}

// updateRelayStatus reflects a relay's claim outcome (claimed routes plus the
// Programmed/Conflict conditions) into its status under an optimistic lock.
func (r *ConfigReconciler) updateRelayStatus(ctx context.Context, relay *v1alpha1.Relay, lost map[string]struct{}) error {
	claimed := claimedRoutes(relay, lost)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current v1alpha1.Relay
		if err := r.Get(ctx, client.ObjectKeyFromObject(relay), &current); err != nil {
			return err
		}
		original := current.DeepCopy()

		current.Status.ClaimedRoutes = claimed
		current.Status.ObservedGeneration = current.Generation

		apimeta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
			Type:               conditionProgrammed,
			Status:             boolCondition(len(claimed) > 0),
			ObservedGeneration: current.Generation,
			Reason:             programmedReason(len(claimed) > 0),
			Message:            programmedMessage(len(claimed)),
		})
		if len(lost) > 0 {
			apimeta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
				Type:               conditionConflict,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: current.Generation,
				Reason:             "RouteClaimed",
				Message:            fmt.Sprintf("%d route(s) already claimed by an earlier relay", len(lost)),
			})
		} else {
			// Conflict is negative-polarity: drop it once the relay is healthy.
			apimeta.RemoveStatusCondition(&current.Status.Conditions, conditionConflict)
		}

		return r.Status().Patch(ctx, &current, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// claimedRoutes returns the recipient-map representation of the routes a relay
// won, sorted for stable status output.
func claimedRoutes(relay *v1alpha1.Relay, lost map[string]struct{}) []string {
	var claimed []string
	for _, route := range relay.Spec.Routes {
		key, repr := postfix.RouteKey(route)
		if key == "" {
			continue
		}
		if _, ok := lost[key]; ok {
			continue
		}
		claimed = append(claimed, repr)
	}
	sort.Strings(claimed)
	return claimed
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range b {
		if a[k] != v {
			return false
		}
	}
	return true
}

func boolCondition(ok bool) metav1.ConditionStatus {
	if ok {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func programmedReason(ok bool) string {
	if ok {
		return "Compiled"
	}
	return "NotCompiled"
}

func programmedMessage(claimed int) string {
	if claimed > 0 {
		return fmt.Sprintf("%d route(s) compiled into the Postfix ingress", claimed)
	}
	return "no routes compiled into the Postfix ingress"
}
