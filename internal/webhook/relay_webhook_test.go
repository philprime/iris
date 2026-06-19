/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package webhook

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/philprime/iris/api/v1alpha1"
)

func scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add v1alpha1: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("add corev1: %v", err)
	}
	return s
}

func transformDestination(cmName, key string) v1alpha1.Destination {
	return v1alpha1.Destination{
		Name: "webhook",
		HTTP: &v1alpha1.HTTPDestination{
			URL:       "https://x.test",
			Transform: &v1alpha1.Transform{JsonnetConfigMapRef: v1alpha1.ConfigMapKeyRef{Name: cmName, Key: key}},
		},
	}
}

func jsonnetConfigMap(name, key, program string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string]string{key: program},
	}
}

// Feature: Relay admission validation
// Scenario: a broken Jsonnet transform is rejected at apply time
func TestValidateRejectsBrokenJsonnet(t *testing.T) {
	cm := jsonnetConfigMap("map", "m.jsonnet", "{ this is broken")
	v := validatorWith(t, cm)
	r := relay("a", []v1alpha1.Route{{Domain: "a.example.com"}}, transformDestination("map", "m.jsonnet"))
	if _, err := v.ValidateCreate(context.Background(), r); err == nil {
		t.Fatal("expected rejection for unparseable jsonnet transform")
	}
}

// Feature: Relay admission validation
// Scenario: a valid Jsonnet transform is admitted
func TestValidateAcceptsValidJsonnet(t *testing.T) {
	cm := jsonnetConfigMap("map", "m.jsonnet", `local e = std.extVar("envelope"); { s: e.subject }`)
	v := validatorWith(t, cm)
	r := relay("a", []v1alpha1.Route{{Domain: "a.example.com"}}, transformDestination("map", "m.jsonnet"))
	if _, err := v.ValidateCreate(context.Background(), r); err != nil {
		t.Fatalf("expected valid transform admitted, got: %v", err)
	}
}

// Feature: Relay admission validation
// Scenario: a transform whose ConfigMap does not exist yet is not blocked
//
//	Given a transform reference with no ConfigMap present
//	When  the relay is validated
//	Then  admission succeeds, deferring the check rather than forcing ordering
func TestValidateSkipsMissingTransformConfigMap(t *testing.T) {
	v := validatorWith(t)
	r := relay("a", []v1alpha1.Route{{Domain: "a.example.com"}}, transformDestination("absent", "m.jsonnet"))
	if _, err := v.ValidateCreate(context.Background(), r); err != nil {
		t.Fatalf("expected admit when transform ConfigMap is absent, got: %v", err)
	}
}

func relay(name string, routes []v1alpha1.Route, dests ...v1alpha1.Destination) *v1alpha1.Relay {
	if dests == nil {
		dests = []v1alpha1.Destination{{Name: "d", HTTP: &v1alpha1.HTTPDestination{URL: "https://x.test"}}}
	}
	return &v1alpha1.Relay{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now())},
		Spec:       v1alpha1.RelaySpec{Routes: routes, Destinations: dests},
	}
}

func validatorWith(t *testing.T, existing ...client.Object) *RelayValidator {
	c := fake.NewClientBuilder().WithScheme(scheme(t)).WithObjects(existing...).Build()
	return &RelayValidator{Client: c}
}

// Feature: Relay admission validation
// Scenario: a destination must be exactly one of http or smtp
//
//	Given a Relay whose destination sets both http and smtp
//	When  it is validated on create
//	Then  admission is rejected
func TestValidateRejectsAmbiguousDestination(t *testing.T) {
	v := validatorWith(t)
	bad := relay("a", []v1alpha1.Route{{Domain: "a.example.com"}}, v1alpha1.Destination{
		Name: "d",
		HTTP: &v1alpha1.HTTPDestination{URL: "https://x.test"},
		SMTP: &v1alpha1.SMTPDestination{Host: "h", Port: 25},
	})
	if _, err := v.ValidateCreate(context.Background(), bad); err == nil {
		t.Fatal("expected error for destination with both http and smtp")
	}
}

// Feature: Relay admission validation
// Scenario: a destination must set at least one delivery method
//
//	Given a Relay whose destination sets neither http nor smtp
//	When  it is validated on create
//	Then  admission is rejected
func TestValidateRejectsEmptyDestination(t *testing.T) {
	v := validatorWith(t)
	bad := relay("a", []v1alpha1.Route{{Domain: "a.example.com"}}, v1alpha1.Destination{Name: "d"})
	if _, err := v.ValidateCreate(context.Background(), bad); err == nil {
		t.Fatal("expected error for destination with neither http nor smtp")
	}
}

// Feature: Relay admission validation
// Scenario: destination names must be unique within a relay
//
//	Given a Relay with two destinations sharing a name
//	When  it is validated on create
//	Then  admission is rejected
func TestValidateRejectsDuplicateDestinationNames(t *testing.T) {
	v := validatorWith(t)
	bad := relay("a", []v1alpha1.Route{{Domain: "a.example.com"}},
		v1alpha1.Destination{Name: "dup", HTTP: &v1alpha1.HTTPDestination{URL: "https://x.test"}},
		v1alpha1.Destination{Name: "dup", SMTP: &v1alpha1.SMTPDestination{Host: "h", Port: 25}},
	)
	if _, err := v.ValidateCreate(context.Background(), bad); err == nil {
		t.Fatal("expected error for duplicate destination names")
	}
}

// Feature: Relay admission validation
// Scenario: a route already claimed by another relay is rejected at apply time
//
//	Given an existing Relay claiming a domain
//	When  a new Relay claiming the same domain is validated on create
//	Then  admission is rejected
func TestValidateRejectsConflictingRoute(t *testing.T) {
	existing := relay("owner", []v1alpha1.Route{{Domain: "shared.example.com"}})
	v := validatorWith(t, existing)
	newcomer := relay("newcomer", []v1alpha1.Route{{Domain: "shared.example.com"}})
	if _, err := v.ValidateCreate(context.Background(), newcomer); err == nil {
		t.Fatal("expected error for route already claimed by another relay")
	}
}

// Feature: Relay admission validation
// Scenario: a valid, non-conflicting Relay is admitted
//
//	Given a Relay with a unique route and a well-formed destination
//	When  it is validated on create
//	Then  admission succeeds
func TestValidateAcceptsValidRelay(t *testing.T) {
	existing := relay("owner", []v1alpha1.Route{{Domain: "owner.example.com"}})
	v := validatorWith(t, existing)
	ok := relay("fresh", []v1alpha1.Route{{Domain: "fresh.example.com"}})
	if _, err := v.ValidateCreate(context.Background(), ok); err != nil {
		t.Fatalf("expected valid relay to be admitted, got: %v", err)
	}
}

// Feature: Relay admission validation
// Scenario: updating a relay does not conflict with its own existing claim
//
//	Given an existing Relay claiming a domain
//	When  the same Relay is updated (still claiming that domain) and validated
//	Then  admission succeeds because a relay never conflicts with itself
func TestValidateUpdateIgnoresSelf(t *testing.T) {
	existing := relay("self", []v1alpha1.Route{{Domain: "self.example.com"}})
	v := validatorWith(t, existing)
	updated := relay("self", []v1alpha1.Route{{Domain: "self.example.com"}, {Domain: "extra.example.com"}})
	if _, err := v.ValidateUpdate(context.Background(), existing, updated); err != nil {
		t.Fatalf("expected self-update to be admitted, got: %v", err)
	}
}
