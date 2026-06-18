/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package postfix

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/philprime/iris/api/v1alpha1"
)

// Feature: Postfix map rendering
// Scenario: a single relay claiming one exact address
//   Given a Relay with a single address route
//   When  the Postfix maps are rendered
//   Then  the transport map routes that address to the relay's Service DNS
func TestRenderSingleExactAddressRouteTransport(t *testing.T) {
	relays := []v1alpha1.Relay{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "appstore-invites", Namespace: "example"},
			Spec: v1alpha1.RelaySpec{
				Routes: []v1alpha1.Route{{Address: "invites@invite.example.com"}},
			},
		},
	}

	maps, _, err := Render(relays, Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	want := "invites@invite.example.com smtp:[relay-appstore-invites.example.svc.cluster.local]:25\n"
	if maps.Transport != want {
		t.Errorf("transport =\n%q\nwant\n%q", maps.Transport, want)
	}
}

// Feature: Postfix map rendering
// Scenario: relay_domains and relay_recipient_maps for an exact address
//   Given a Relay with a single address route
//   When  the Postfix maps are rendered
//   Then  the address domain is a relay domain and the address is an allowed recipient
func TestRenderSingleExactAddressRouteDomainsAndRecipients(t *testing.T) {
	relays := []v1alpha1.Relay{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "appstore-invites", Namespace: "example"},
			Spec: v1alpha1.RelaySpec{
				Routes: []v1alpha1.Route{{Address: "invites@invite.example.com"}},
			},
		},
	}

	maps, _, err := Render(relays, Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if want := "invite.example.com\n"; maps.RelayDomains != want {
		t.Errorf("relayDomains =\n%q\nwant\n%q", maps.RelayDomains, want)
	}
	if want := "invites@invite.example.com OK\n"; maps.RelayRecipients != want {
		t.Errorf("relayRecipients =\n%q\nwant\n%q", maps.RelayRecipients, want)
	}
}

// Feature: Postfix map rendering
// Scenario: a relay claiming a whole domain
//   Given a Relay with a single domain route
//   When  the Postfix maps are rendered
//   Then  the domain routes to the Service DNS and any local-part is an allowed recipient
func TestRenderSingleDomainRoute(t *testing.T) {
	relays := []v1alpha1.Relay{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "appstore-invites", Namespace: "example"},
			Spec: v1alpha1.RelaySpec{
				Routes: []v1alpha1.Route{{Domain: "invite.example.com"}},
			},
		},
	}

	maps, _, err := Render(relays, Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if want := "invite.example.com smtp:[relay-appstore-invites.example.svc.cluster.local]:25\n"; maps.Transport != want {
		t.Errorf("transport =\n%q\nwant\n%q", maps.Transport, want)
	}
	if want := "invite.example.com\n"; maps.RelayDomains != want {
		t.Errorf("relayDomains =\n%q\nwant\n%q", maps.RelayDomains, want)
	}
	if want := "@invite.example.com OK\n"; maps.RelayRecipients != want {
		t.Errorf("relayRecipients =\n%q\nwant\n%q", maps.RelayRecipients, want)
	}
}

// Feature: Postfix map rendering
// Scenario: output is stable regardless of input order
//   Given two relays supplied in reverse-sorted order
//   When  the Postfix maps are rendered
//   Then  every map is sorted by route key so the output is byte-stable
func TestRenderSortsOutputByRouteKey(t *testing.T) {
	relays := []v1alpha1.Relay{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "z", Namespace: "ns"},
			Spec: v1alpha1.RelaySpec{
				Routes: []v1alpha1.Route{{Address: "zeta@z.example.com"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
			Spec: v1alpha1.RelaySpec{
				Routes: []v1alpha1.Route{{Domain: "a.example.com"}},
			},
		},
	}

	maps, _, err := Render(relays, Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	wantTransport := "a.example.com smtp:[relay-a.ns.svc.cluster.local]:25\n" +
		"zeta@z.example.com smtp:[relay-z.ns.svc.cluster.local]:25\n"
	if maps.Transport != wantTransport {
		t.Errorf("transport =\n%q\nwant\n%q", maps.Transport, wantTransport)
	}

	wantDomains := "a.example.com\nz.example.com\n"
	if maps.RelayDomains != wantDomains {
		t.Errorf("relayDomains =\n%q\nwant\n%q", maps.RelayDomains, wantDomains)
	}

	wantRecipients := "@a.example.com OK\nzeta@z.example.com OK\n"
	if maps.RelayRecipients != wantRecipients {
		t.Errorf("relayRecipients =\n%q\nwant\n%q", maps.RelayRecipients, wantRecipients)
	}
}

// Feature: Postfix map rendering
// Scenario: two relays claim the same route key
//   Given an incumbent relay created before a newcomer, both claiming one address
//   When  the Postfix maps are rendered
//   Then  the incumbent owns the route and the newcomer is reported as a conflict
func TestRenderFirstWriterWinsOnConflict(t *testing.T) {
	early := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	late := metav1.NewTime(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	// Supplied newcomer-first to prove ordering is by creationTimestamp, not
	// input order.
	relays := []v1alpha1.Relay{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "newcomer", Namespace: "ns", CreationTimestamp: late},
			Spec:       v1alpha1.RelaySpec{Routes: []v1alpha1.Route{{Address: "shared@x.example.com"}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "incumbent", Namespace: "ns", CreationTimestamp: early},
			Spec:       v1alpha1.RelaySpec{Routes: []v1alpha1.Route{{Address: "shared@x.example.com"}}},
		},
	}

	maps, conflicts, err := Render(relays, Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	// The incumbent owns the transport target and is the only allowed recipient.
	if want := "shared@x.example.com smtp:[relay-incumbent.ns.svc.cluster.local]:25\n"; maps.Transport != want {
		t.Errorf("transport =\n%q\nwant\n%q", maps.Transport, want)
	}

	if len(conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", len(conflicts), conflicts)
	}
	c := conflicts[0]
	if c.Relay.Name != "newcomer" || c.Relay.Namespace != "ns" {
		t.Errorf("conflict relay = %v, want ns/newcomer", c.Relay)
	}
	if c.Route != "shared@x.example.com" {
		t.Errorf("conflict route = %q, want shared@x.example.com", c.Route)
	}
	if c.WonBy.Name != "incumbent" {
		t.Errorf("conflict wonBy = %v, want ns/incumbent", c.WonBy)
	}
}

// Feature: Postfix map rendering
// Scenario: a conflict between relays with equal creation timestamps
//   Given two relays created at the same instant, both claiming one address
//   When  the Postfix maps are rendered
//   Then  the tie breaks deterministically by namespace then name
func TestRenderConflictTieBreaksByNamespaceThenName(t *testing.T) {
	ts := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// Same namespace, same timestamp: lower name ("a") wins. Supplied b-first.
	relays := []v1alpha1.Relay{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns", CreationTimestamp: ts},
			Spec:       v1alpha1.RelaySpec{Routes: []v1alpha1.Route{{Address: "shared@x.example.com"}}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns", CreationTimestamp: ts},
			Spec:       v1alpha1.RelaySpec{Routes: []v1alpha1.Route{{Address: "shared@x.example.com"}}},
		},
	}

	maps, conflicts, err := Render(relays, Options{})
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	if want := "shared@x.example.com smtp:[relay-a.ns.svc.cluster.local]:25\n"; maps.Transport != want {
		t.Errorf("transport =\n%q\nwant\n%q", maps.Transport, want)
	}
	if len(conflicts) != 1 || conflicts[0].Relay.Name != "b" || conflicts[0].WonBy.Name != "a" {
		t.Fatalf("conflicts = %+v, want b losing to a", conflicts)
	}
}
