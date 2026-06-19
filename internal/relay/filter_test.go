/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"testing"

	"github.com/philprime/iris/api/v1alpha1"
)

func appleMessage(extraHeaders string) []byte {
	return []byte("From: Apple <noreply@email.apple.com>\r\n" +
		"To: invites@invite.example.com\r\n" +
		"Message-ID: <m1@email.apple.com>\r\n" +
		extraHeaders +
		"Subject: invite\r\n\r\nVisit https://email.apple.com/x for details\r\n")
}

// Feature: inbound filtering
// Scenario: with no filters configured every message is accepted
func TestEvaluateNoFiltersAccepts(t *testing.T) {
	d := Evaluate(nil, "noreply@email.apple.com", appleMessage(""), 100)
	if !d.Accept {
		t.Errorf("expected accept with no filters, got %+v", d)
	}
}

// Feature: inbound filtering
// Scenario: a message larger than maxMessageBytes is rejected
func TestEvaluateRejectsOversize(t *testing.T) {
	filters := &v1alpha1.Filters{MaxMessageBytes: 10}
	d := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 1000)
	if d.Accept || d.Reason != ReasonSize {
		t.Errorf("expected size rejection, got %+v", d)
	}
}

// Feature: inbound filtering
// Scenario: a sender outside allowedSenderDomains is rejected
func TestEvaluateRejectsDisallowedSender(t *testing.T) {
	filters := &v1alpha1.Filters{AllowedSenderDomains: []string{"email.apple.com"}}
	d := Evaluate(filters, "attacker@evil.example", appleMessage(""), 100)
	if d.Accept || d.Reason != ReasonSender {
		t.Errorf("expected sender rejection, got %+v", d)
	}
}

// Feature: inbound filtering
// Scenario: requireDKIM rejects a message without a matching signature
func TestEvaluateRequireDKIM(t *testing.T) {
	filters := &v1alpha1.Filters{RequireDKIM: []string{"email.apple.com"}}

	without := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 100)
	if without.Accept || without.Reason != ReasonDKIM {
		t.Errorf("expected dkim rejection without signature, got %+v", without)
	}

	dkim := "DKIM-Signature: v=1; a=rsa-sha256; d=email.apple.com; s=sel; bh=x; b=y\r\n"
	with := Evaluate(filters, "noreply@email.apple.com", appleMessage(dkim), 100)
	if !with.Accept {
		t.Errorf("expected accept with matching dkim signature, got %+v", with)
	}
}

// Feature: inbound scoring
// Scenario: the heuristic score gates acceptance against minScore
//
//	Given fromDomain and messageIdDomain both matching an allowed domain
//	When  minScore is 2 the message is accepted, when 3 it is rejected
func TestEvaluateScoreGate(t *testing.T) {
	base := v1alpha1.Filters{
		AllowedSenderDomains: []string{"email.apple.com"},
		ScoreSignals: []v1alpha1.ScoreSignal{
			v1alpha1.ScoreSignalFromDomain,
			v1alpha1.ScoreSignalMessageIDDomain,
		},
	}

	accept := base
	accept.MinScore = 2
	if d := Evaluate(&accept, "noreply@email.apple.com", appleMessage(""), 100); !d.Accept || d.Score != 2 {
		t.Errorf("expected accept with score 2, got %+v", d)
	}

	reject := base
	reject.MinScore = 3
	if d := Evaluate(&reject, "noreply@email.apple.com", appleMessage(""), 100); d.Accept || d.Reason != ReasonScore {
		t.Errorf("expected score rejection at minScore 3, got %+v", d)
	}
}

// Feature: inbound scoring
// Scenario: the bodyLinkDomain signal matches a link to an allowed domain
func TestEvaluateBodyLinkSignal(t *testing.T) {
	filters := &v1alpha1.Filters{
		AllowedSenderDomains: []string{"email.apple.com"},
		ScoreSignals:         []v1alpha1.ScoreSignal{v1alpha1.ScoreSignalBodyLinkDomain},
		MinScore:             1,
	}
	if d := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 100); !d.Accept || d.Score != 1 {
		t.Errorf("expected body-link signal to score 1 and accept, got %+v", d)
	}
}
