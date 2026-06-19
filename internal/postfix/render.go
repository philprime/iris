/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

// Package postfix renders the aggregate Postfix routing maps (transport,
// relay_domains, relay_recipient_maps) from the set of Relay resources.
package postfix

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"github.com/philprime/iris/api/v1alpha1"
)

// Options tunes how routes are rendered into Postfix map targets.
type Options struct {
	// ClusterDomain is the in-cluster DNS suffix. Defaults to cluster.local.
	ClusterDomain string
	// SMTPPort is the port relays listen on for SMTP. Defaults to 25.
	SMTPPort int
}

// Maps holds the rendered contents of the three Postfix map files.
type Maps struct {
	Transport       string
	RelayDomains    string
	RelayRecipients string
}

// Conflict reports a route key that a relay could not claim because an
// earlier relay already owns it. The caller uses this to set Conflict=True on
// the losing relay.
type Conflict struct {
	// Relay is the losing relay (the later claimant).
	Relay types.NamespacedName
	// Route is the route key (exact address or domain) that collided.
	Route string
	// WonBy is the relay that owns the route key.
	WonBy types.NamespacedName
}

// Render compiles the Postfix maps from the given relays. Route keys are
// unique cluster-wide: on a collision the earliest relay by (creationTimestamp,
// namespace, name) wins and the later claimants are returned as conflicts.
func Render(relays []v1alpha1.Relay, opts Options) (Maps, []Conflict, error) {
	clusterDomain := opts.ClusterDomain
	if clusterDomain == "" {
		clusterDomain = "cluster.local"
	}
	port := opts.SMTPPort
	if port == 0 {
		port = 25
	}

	// Resolve route ownership in first-writer-wins order so a collision is
	// decided deterministically and independently of input order.
	ordered := make([]v1alpha1.Relay, len(relays))
	copy(ordered, relays)
	sort.SliceStable(ordered, func(i, j int) bool { return lessRelay(ordered[i], ordered[j]) })

	// Collect the map lines keyed by their Postfix lookup key. Sorting happens
	// at render time so the output is byte-stable regardless of input order.
	transport := map[string]string{}
	recipients := map[string]string{}
	domains := map[string]struct{}{}
	owner := map[string]types.NamespacedName{}
	var conflicts []Conflict

	for _, relay := range ordered {
		nn := types.NamespacedName{Namespace: relay.Namespace, Name: relay.Name}
		target := fmt.Sprintf("smtp:[relay-%s.%s.svc.%s]:%d", relay.Name, relay.Namespace, clusterDomain, port)
		for _, route := range relay.Spec.Routes {
			key, recipient, domain := routeKeys(route)
			if key == "" {
				continue
			}
			if won, claimed := owner[key]; claimed {
				conflicts = append(conflicts, Conflict{Relay: nn, Route: key, WonBy: won})
				continue
			}
			owner[key] = nn
			transport[key] = target
			recipients[recipient] = "OK"
			domains[domain] = struct{}{}
		}
	}

	return Maps{
		Transport:       renderPairs(transport),
		RelayRecipients: renderPairs(recipients),
		RelayDomains:    renderKeys(domains),
	}, conflicts, nil
}

// RouteKey returns the cluster-wide conflict key and the relay_recipient_maps
// representation (the form surfaced in status.claimedRoutes) for a route. Both
// are empty when the route sets neither address nor domain. The conflict key is
// what Render dedups on and what a Conflict reports in its Route field.
func RouteKey(route v1alpha1.Route) (conflictKey, claimed string) {
	key, recipient, _ := routeKeys(route)
	return key, recipient
}

// routeKeys returns the conflict/transport key, the relay_recipient_maps key,
// and the relay domain for a route. An exact address and a whole-domain route
// occupy different key spaces (an address always contains '@'), so they never
// collide with each other.
func routeKeys(route v1alpha1.Route) (key, recipient, domain string) {
	switch {
	case route.Address != "":
		domain = route.Address
		if at := strings.IndexByte(route.Address, '@'); at >= 0 {
			domain = route.Address[at+1:]
		}
		return route.Address, route.Address, domain
	case route.Domain != "":
		return route.Domain, "@" + route.Domain, route.Domain
	default:
		return "", "", ""
	}
}

// lessRelay orders relays by creationTimestamp, then namespace, then name, so
// the earliest claimant wins a contested route key.
func lessRelay(a, b v1alpha1.Relay) bool {
	at, bt := a.CreationTimestamp.Time, b.CreationTimestamp.Time
	if !at.Equal(bt) {
		return at.Before(bt)
	}
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	return a.Name < b.Name
}

// renderPairs writes "key value" lines sorted by key.
func renderPairs(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %s\n", k, m[k])
	}
	return b.String()
}

// renderKeys writes sorted, deduplicated keys one per line.
func renderKeys(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s\n", k)
	}
	return b.String()
}
