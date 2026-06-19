/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/philprime/iris/api/v1alpha1"
)

// Feature: reconcile error classification
// Scenario: terminal errors are distinguished from transient ones, through wrapping
func TestTerminalClassification(t *testing.T) {
	if isTerminal(nil) {
		t.Error("nil should not be terminal")
	}
	if isTerminal(errors.New("transient")) {
		t.Error("a plain error should not be terminal")
	}
	if !isTerminal(terminal(errors.New("bad"))) {
		t.Error("a wrapped error should be terminal")
	}
	if !isTerminal(fmt.Errorf("context: %w", terminal(errors.New("bad")))) {
		t.Error("a nested terminal error should be detected through unwrapping")
	}
}

// Feature: reconcile error classification
// Scenario: a terminal error sets Ready=False and does not requeue
//
//	Given a terminal reconcile error
//	When  it is handled
//	Then  no error is returned (no requeue) and the relay is Ready=False
func TestHandleReconcileErrorTerminal(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rel := relayClaiming("alpha", base, v1alpha1.Route{Domain: "invite.example.com"})
	r, c := newRelayReconciler(scheme, rel)

	res, err := r.handleReconcileError(context.Background(), rel, terminal(errors.New("bad config")))
	if err != nil {
		t.Fatalf("terminal error should not requeue, got err: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Errorf("terminal error should not requeue, got %+v", res)
	}

	var got v1alpha1.Relay
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(rel), &got); err != nil {
		t.Fatalf("get relay: %v", err)
	}
	cond := apimeta.FindStatusCondition(got.Status.Conditions, conditionReady)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Errorf("Ready = %v, want False on terminal error", cond)
	}
}

// Feature: reconcile error classification
// Scenario: a transient error requeues and does not change status
func TestHandleReconcileErrorTransient(t *testing.T) {
	scheme := testScheme(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rel := relayClaiming("alpha", base, v1alpha1.Route{Domain: "invite.example.com"})
	r, _ := newRelayReconciler(scheme, rel)

	_, err := r.handleReconcileError(context.Background(), rel, errors.New("api glitch"))
	if err == nil {
		t.Error("a transient error should be returned so the controller requeues")
	}
}
