/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package v1alpha1 contains the API schema definitions for the iris
// v1alpha1 API group (iris.philprime.dev).
// +kubebuilder:object:generate=true
// +groupName=iris.philprime.dev
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "iris.philprime.dev", Version: "v1alpha1"}

	// SchemeBuilder collects the functions that register this group-version's
	// types with a runtime.Scheme. It depends only on apimachinery so the API
	// package stays cheap to import.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &Relay{}, &RelayList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
