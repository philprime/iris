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
	"strings"

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

// Render compiles the Postfix maps from the given relays.
func Render(relays []v1alpha1.Relay, opts Options) (Maps, error) {
	clusterDomain := opts.ClusterDomain
	if clusterDomain == "" {
		clusterDomain = "cluster.local"
	}
	port := opts.SMTPPort
	if port == 0 {
		port = 25
	}

	var transport, recipients, relayDomains strings.Builder
	seenDomain := map[string]bool{}
	addDomain := func(domain string) {
		if domain == "" || seenDomain[domain] {
			return
		}
		seenDomain[domain] = true
		fmt.Fprintf(&relayDomains, "%s\n", domain)
	}

	for _, relay := range relays {
		target := fmt.Sprintf("smtp:[relay-%s.%s.svc.%s]:%d", relay.Name, relay.Namespace, clusterDomain, port)
		for _, route := range relay.Spec.Routes {
			switch {
			case route.Address != "":
				fmt.Fprintf(&transport, "%s %s\n", route.Address, target)
				fmt.Fprintf(&recipients, "%s OK\n", route.Address)
				if at := strings.IndexByte(route.Address, '@'); at >= 0 {
					addDomain(route.Address[at+1:])
				}
			case route.Domain != "":
				fmt.Fprintf(&transport, "%s %s\n", route.Domain, target)
				fmt.Fprintf(&recipients, "@%s OK\n", route.Domain)
				addDomain(route.Domain)
			}
		}
	}

	return Maps{
		Transport:       transport.String(),
		RelayDomains:    relayDomains.String(),
		RelayRecipients: recipients.String(),
	}, nil
}
