/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package postfix

import (
	"testing"

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

	maps, err := Render(relays, Options{})
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

	maps, err := Render(relays, Options{})
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

	maps, err := Render(relays, Options{})
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
