/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package controller

import (
	"context"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/philprime/iris/api/v1alpha1"
	"github.com/philprime/iris/internal/relay"
)

// relayFinalizer is held on a Relay so its routes release from the aggregate
// before the object is deleted.
const relayFinalizer = "iris.philprime.dev/finalizer"

// conditionDeploymentAvailable reports the transformer Deployment's readiness.
const conditionDeploymentAvailable = "DeploymentAvailable"

// Ports exposed by every relay pod and Service.
const (
	smtpPort  int32 = 25
	adminPort int32 = 8080
	// relayMountDir is the base path the relay reads. The config, referenced
	// secrets, and Jsonnet transforms mount as siblings under it, matching the
	// layout internal/relay.BuildTargets expects.
	relayMountDir = "/etc/iris/relay"
)

// RelayReconciler reconciles the per-relay Deployment, Service, and config
// ConfigMap, and owns the per-relay status (serviceRef, DeploymentAvailable,
// and the Ready summary).
type RelayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// RelayImage is the container image used for the transformer pod.
	RelayImage string
}

// SetupWithManager registers the RelayReconciler and the child resources it
// owns so updates to them re-trigger reconciliation.
func (r *RelayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Relay{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Named("relay").
		Complete(r)
}

// Reconcile ensures a Relay's children exist and reflects their state.

//+kubebuilder:rbac:groups=iris.philprime.dev,resources=relays,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=iris.philprime.dev,resources=relays/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=iris.philprime.dev,resources=relays/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *RelayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	var rel v1alpha1.Relay
	if err := r.Get(ctx, req.NamespacedName, &rel); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !rel.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &rel)
	}

	if !controllerutil.ContainsFinalizer(&rel, relayFinalizer) {
		patch := client.MergeFrom(rel.DeepCopy())
		controllerutil.AddFinalizer(&rel, relayFinalizer)
		if err := r.Patch(ctx, &rel, patch); err != nil {
			return reconcile.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	if err := r.reconcileConfigMap(ctx, &rel); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.reconcileService(ctx, &rel); err != nil {
		return reconcile.Result{}, err
	}
	dep, err := r.reconcileDeployment(ctx, &rel)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.updateStatus(ctx, &rel, dep); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// reconcileDelete releases the finalizer so the API server can complete the
// deletion. Children are garbage-collected by their owner references, and the
// ConfigReconciler re-renders the Postfix maps without this relay.
func (r *RelayReconciler) reconcileDelete(ctx context.Context, rel *v1alpha1.Relay) (reconcile.Result, error) {
	if !controllerutil.ContainsFinalizer(rel, relayFinalizer) {
		return reconcile.Result{}, nil
	}
	patch := client.MergeFrom(rel.DeepCopy())
	controllerutil.RemoveFinalizer(rel, relayFinalizer)
	if err := r.Patch(ctx, rel, patch); err != nil {
		return reconcile.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}
	return reconcile.Result{}, nil
}

func (r *RelayReconciler) reconcileConfigMap(ctx context.Context, rel *v1alpha1.Relay) error {
	rendered, err := relay.RenderConfig(rel)
	if err != nil {
		return fmt.Errorf("render relay config: %w", err)
	}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: configMapName(rel.Name), Namespace: rel.Namespace}}
	return r.apply(ctx, rel, cm, func() error {
		cm.Labels = relayLabels(rel.Name)
		cm.Data = map[string]string{relay.ConfigFileName: string(rendered)}
		return nil
	})
}

func (r *RelayReconciler) reconcileService(ctx context.Context, rel *v1alpha1.Relay) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: childName(rel.Name), Namespace: rel.Namespace}}
	return r.apply(ctx, rel, svc, func() error {
		svc.Labels = relayLabels(rel.Name)
		svc.Spec.Selector = relayLabels(rel.Name)
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Ports = []corev1.ServicePort{
			{Name: "smtp", Port: smtpPort, TargetPort: intstr.FromInt32(smtpPort), Protocol: corev1.ProtocolTCP},
			{Name: "metrics", Port: adminPort, TargetPort: intstr.FromInt32(adminPort), Protocol: corev1.ProtocolTCP},
		}
		return nil
	})
}

func (r *RelayReconciler) reconcileDeployment(ctx context.Context, rel *v1alpha1.Relay) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: childName(rel.Name), Namespace: rel.Namespace}}
	err := r.apply(ctx, rel, dep, func() error {
		labels := relayLabels(rel.Name)
		dep.Labels = labels
		dep.Spec.Replicas = relayReplicas(rel)
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		dep.Spec.Template.ObjectMeta.Labels = labels
		volumes, mounts := relayVolumes(rel)
		dep.Spec.Template.Spec = corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "relay",
				Image: r.RelayImage,
				Ports: []corev1.ContainerPort{
					{Name: "smtp", ContainerPort: smtpPort, Protocol: corev1.ProtocolTCP},
					{Name: "admin", ContainerPort: adminPort, Protocol: corev1.ProtocolTCP},
				},
				Env: []corev1.EnvVar{
					{Name: "IRIS_RELAY_CONFIG", Value: relayMountDir + "/config/" + relay.ConfigFileName},
					{Name: "IRIS_RELAY_MOUNT_DIR", Value: relayMountDir},
				},
				Resources:    relayResources(rel),
				VolumeMounts: mounts,
			}},
			Volumes: volumes,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dep, nil
}

// relayVolumes builds the pod volumes and container mounts for a relay: its
// rendered config, plus every referenced auth Secret and Jsonnet ConfigMap,
// mounted where internal/relay.BuildTargets reads them. The relay pod has no
// Kubernetes API access, so these inputs must be projected as files.
func relayVolumes(rel *v1alpha1.Relay) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMapName(rel.Name)},
			},
		},
	}}
	mounts := []corev1.VolumeMount{
		{Name: "config", MountPath: relayMountDir + "/config", ReadOnly: true},
	}

	secrets, configMaps := referencedMounts(rel)
	for _, name := range secrets {
		volumes = append(volumes, corev1.Volume{
			Name:         "secret-" + name,
			VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name}},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "secret-" + name,
			MountPath: relayMountDir + "/secrets/" + name,
			ReadOnly:  true,
		})
	}
	for _, name := range configMaps {
		volumes = append(volumes, corev1.Volume{
			Name: "transform-" + name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "transform-" + name,
			MountPath: relayMountDir + "/transforms/" + name,
			ReadOnly:  true,
		})
	}
	return volumes, mounts
}

// referencedMounts returns the sorted, de-duplicated names of the Secrets and
// Jsonnet ConfigMaps a relay's destinations reference.
func referencedMounts(rel *v1alpha1.Relay) (secrets, configMaps []string) {
	secretSet := map[string]struct{}{}
	configMapSet := map[string]struct{}{}
	for _, dest := range rel.Spec.Destinations {
		if dest.HTTP != nil {
			if dest.HTTP.AuthSecretRef != nil {
				secretSet[dest.HTTP.AuthSecretRef.Name] = struct{}{}
			}
			if dest.HTTP.Transform != nil {
				configMapSet[dest.HTTP.Transform.JsonnetConfigMapRef.Name] = struct{}{}
			}
		}
		if dest.SMTP != nil && dest.SMTP.AuthSecretRef != nil {
			secretSet[dest.SMTP.AuthSecretRef.Name] = struct{}{}
		}
	}
	return sortedKeys(secretSet), sortedKeys(configMapSet)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// apply creates or updates a child object owned by the relay, setting the
// controller reference and applying the caller's desired state via mutate.
func (r *RelayReconciler) apply(ctx context.Context, rel *v1alpha1.Relay, obj client.Object, mutate func() error) error {
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, func() error {
		if err := mutate(); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(rel, obj, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("apply %T %s: %w", obj, obj.GetName(), err)
	}
	return nil
}

// updateStatus reflects the Service reference and the Deployment's availability
// into the relay's status, then summarizes Ready.
func (r *RelayReconciler) updateStatus(ctx context.Context, rel *v1alpha1.Relay, dep *appsv1.Deployment) error {
	available := deploymentAvailable(dep)

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var current v1alpha1.Relay
		if err := r.Get(ctx, client.ObjectKeyFromObject(rel), &current); err != nil {
			return err
		}
		original := current.DeepCopy()

		current.Status.ServiceRef = &v1alpha1.ServiceReference{Name: childName(rel.Name)}
		current.Status.ObservedGeneration = current.Generation

		apimeta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
			Type:               conditionDeploymentAvailable,
			Status:             boolCondition(available),
			ObservedGeneration: current.Generation,
			Reason:             deploymentReason(available),
			Message:            deploymentMessage(available),
		})
		apimeta.SetStatusCondition(&current.Status.Conditions, summarizeReady(&current))

		return r.Status().Patch(ctx, &current, client.MergeFromWithOptions(original, client.MergeFromWithOptimisticLock{}))
	})
}

// summarizeReady computes the Ready condition from the owned conditions: a relay
// is Ready when it is Programmed, not in Conflict, and its Deployment is
// Available.
func summarizeReady(rel *v1alpha1.Relay) metav1.Condition {
	programmed := apimeta.IsStatusConditionTrue(rel.Status.Conditions, conditionProgrammed)
	conflict := apimeta.IsStatusConditionTrue(rel.Status.Conditions, conditionConflict)
	available := apimeta.IsStatusConditionTrue(rel.Status.Conditions, conditionDeploymentAvailable)

	ready := programmed && !conflict && available
	cond := metav1.Condition{
		Type:               conditionReady,
		Status:             boolCondition(ready),
		ObservedGeneration: rel.Generation,
		Reason:             "Ready",
		Message:            "relay is programmed and its transformer is available",
	}
	switch {
	case conflict:
		cond.Reason = "Conflict"
		cond.Message = "relay lost one or more routes to an earlier claimant"
	case !programmed:
		cond.Reason = "NotProgrammed"
		cond.Message = "relay routes are not yet compiled into the Postfix ingress"
	case !available:
		cond.Reason = "DeploymentUnavailable"
		cond.Message = "the transformer Deployment is not yet available"
	}
	return cond
}

func deploymentAvailable(dep *appsv1.Deployment) bool {
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable {
			return c.Status == corev1.ConditionTrue
		}
	}
	return dep.Status.AvailableReplicas > 0
}

func childName(relayName string) string     { return "relay-" + relayName }
func configMapName(relayName string) string { return childName(relayName) + "-config" }

func relayLabels(relayName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "iris-relay",
		"app.kubernetes.io/managed-by": "iris",
		"iris.philprime.dev/relay":     relayName,
	}
}

func relayReplicas(rel *v1alpha1.Relay) *int32 {
	if rel.Spec.Deployment != nil && rel.Spec.Deployment.Replicas != nil {
		return rel.Spec.Deployment.Replicas
	}
	one := int32(1)
	return &one
}

func relayResources(rel *v1alpha1.Relay) corev1.ResourceRequirements {
	if rel.Spec.Deployment != nil {
		return rel.Spec.Deployment.Resources
	}
	return corev1.ResourceRequirements{}
}

func deploymentReason(available bool) string {
	if available {
		return "MinimumReplicasAvailable"
	}
	return "Progressing"
}

func deploymentMessage(available bool) string {
	if available {
		return "the transformer Deployment has available replicas"
	}
	return "the transformer Deployment has no available replicas yet"
}
