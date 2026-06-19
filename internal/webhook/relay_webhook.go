/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package webhook holds the validating admission webhook for Relay resources.
// It is a backstop for the CRD's CEL validations plus the cluster-wide route
// conflict pre-check that CEL cannot express.
package webhook

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/philprime/iris/api/v1alpha1"
	"github.com/philprime/iris/internal/postfix"
)

// SetupRelayWebhookWithManager registers the validating webhook for Relay on
// the manager's webhook server.

//+kubebuilder:webhook:path=/validate-iris-philprime-dev-v1alpha1-relay,mutating=false,failurePolicy=fail,sideEffects=None,groups=iris.philprime.dev,resources=relays,verbs=create;update,versions=v1alpha1,name=vrelay.iris.philprime.dev,admissionReviewVersions=v1

func SetupRelayWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &v1alpha1.Relay{}).
		WithValidator(&RelayValidator{Client: mgr.GetClient()}).
		Complete()
}

// RelayValidator validates Relay resources on admission.
type RelayValidator struct {
	// Client reads existing Relays for the cluster-wide route conflict pre-check.
	Client client.Reader
}

var _ admission.Validator[*v1alpha1.Relay] = &RelayValidator{}

// ValidateCreate validates a Relay being created.
func (v *RelayValidator) ValidateCreate(ctx context.Context, relay *v1alpha1.Relay) (admission.Warnings, error) {
	return nil, v.validate(ctx, relay)
}

// ValidateUpdate validates a Relay being updated.
func (v *RelayValidator) ValidateUpdate(ctx context.Context, _ *v1alpha1.Relay, newRelay *v1alpha1.Relay) (admission.Warnings, error) {
	return nil, v.validate(ctx, newRelay)
}

// ValidateDelete allows any delete. Route release is handled by the finalizer.
func (v *RelayValidator) ValidateDelete(_ context.Context, _ *v1alpha1.Relay) (admission.Warnings, error) {
	return nil, nil
}

// validate runs the structural checks and the cluster-wide conflict pre-check,
// aggregating every violation into a single error.
func (v *RelayValidator) validate(ctx context.Context, relay *v1alpha1.Relay) error {
	var errs field.ErrorList
	errs = append(errs, validateDestinations(relay)...)

	conflicts, err := v.conflictingRoutes(ctx, relay)
	if err != nil {
		return fmt.Errorf("check route conflicts: %w", err)
	}
	errs = append(errs, conflicts...)

	if len(errs) == 0 {
		return nil
	}
	gk := v1alpha1.GroupVersion.WithKind("Relay").GroupKind()
	return apierrors.NewInvalid(gk, relay.Name, errs)
}

// validateDestinations enforces the destination union (exactly one of http or
// smtp) and destination name uniqueness within the relay.
func validateDestinations(relay *v1alpha1.Relay) field.ErrorList {
	var errs field.ErrorList
	base := field.NewPath("spec", "destinations")
	seen := map[string]struct{}{}
	for i, d := range relay.Spec.Destinations {
		p := base.Index(i)
		set := 0
		if d.HTTP != nil {
			set++
		}
		if d.SMTP != nil {
			set++
		}
		if set != 1 {
			errs = append(errs, field.Invalid(p, d.Name, "exactly one of http or smtp must be set"))
		}
		if _, dup := seen[d.Name]; dup {
			errs = append(errs, field.Duplicate(p.Child("name"), d.Name))
		}
		seen[d.Name] = struct{}{}
	}
	return errs
}

// conflictingRoutes returns an error for every route key the relay claims that
// another existing relay already owns.
func (v *RelayValidator) conflictingRoutes(ctx context.Context, relay *v1alpha1.Relay) (field.ErrorList, error) {
	var existing v1alpha1.RelayList
	if err := v.Client.List(ctx, &existing); err != nil {
		return nil, err
	}

	owned := map[string]string{}
	for i := range existing.Items {
		other := &existing.Items[i]
		if other.Namespace == relay.Namespace && other.Name == relay.Name {
			continue // a relay never conflicts with itself
		}
		for _, route := range other.Spec.Routes {
			if key, _ := postfix.RouteKey(route); key != "" {
				owned[key] = other.Namespace + "/" + other.Name
			}
		}
	}

	var errs field.ErrorList
	base := field.NewPath("spec", "routes")
	for i, route := range relay.Spec.Routes {
		key, _ := postfix.RouteKey(route)
		if key == "" {
			continue
		}
		if by, taken := owned[key]; taken {
			errs = append(errs, field.Invalid(base.Index(i), key, fmt.Sprintf("route already claimed by relay %s", by)))
		}
	}
	return errs, nil
}
