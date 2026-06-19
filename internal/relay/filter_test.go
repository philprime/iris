/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/emersion/go-msgauth/dkim"

	"github.com/philprime/iris/api/v1alpha1"
)

func appleMessage(extraHeaders string) []byte {
	return []byte("From: Apple <noreply@email.apple.com>\r\n" +
		"To: invites@invite.example.com\r\n" +
		"Message-ID: <m1@email.apple.com>\r\n" +
		extraHeaders +
		"Subject: invite\r\n\r\nVisit https://email.apple.com/x for details\r\n")
}

// testKeyBits keeps RSA generation cheap. DKIM requires at least 1024 bits.
const testKeyBits = 1024

// signMessage returns the message signed with a DKIM-Signature header for the
// given domain and selector.
func signMessage(t *testing.T, priv *rsa.PrivateKey, domain, selector string, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	opts := &dkim.SignOptions{
		Domain:   domain,
		Selector: selector,
		Signer:   priv,
		Hash:     crypto.SHA256,
	}
	if err := dkim.Sign(&buf, bytes.NewReader(raw), opts); err != nil {
		t.Fatalf("sign message: %v", err)
	}
	return buf.Bytes()
}

// txtResolver returns a DKIMResolver that serves the public key for one
// selector and domain, so verification never touches DNS.
func txtResolver(t *testing.T, priv *rsa.PrivateKey, domain, selector string) DKIMResolver {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	record := "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(der)
	want := selector + "._domainkey." + domain
	return func(name string) ([]string, error) {
		if strings.TrimSuffix(name, ".") == want {
			return []string{record}, nil
		}
		return nil, fmt.Errorf("no TXT record for %s", name)
	}
}

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, testKeyBits)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return priv
}

// Feature: inbound filtering
// Scenario: with no filters configured every message is accepted
func TestEvaluateNoFiltersAccepts(t *testing.T) {
	d := Evaluate(nil, "noreply@email.apple.com", appleMessage(""), 100, nil)
	if !d.Accept {
		t.Errorf("expected accept with no filters, got %+v", d)
	}
}

// Feature: inbound filtering
// Scenario: a message larger than maxMessageBytes is rejected
func TestEvaluateRejectsOversize(t *testing.T) {
	filters := &v1alpha1.Filters{MaxMessageBytes: 10}
	d := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 1000, nil)
	if d.Accept || d.Reason != ReasonSize {
		t.Errorf("expected size rejection, got %+v", d)
	}
}

// Feature: inbound filtering
// Scenario: a sender outside allowedSenderDomains is rejected
func TestEvaluateRejectsDisallowedSender(t *testing.T) {
	filters := &v1alpha1.Filters{AllowedSenderDomains: []string{"email.apple.com"}}
	d := Evaluate(filters, "attacker@evil.example", appleMessage(""), 100, nil)
	if d.Accept || d.Reason != ReasonSender {
		t.Errorf("expected sender rejection, got %+v", d)
	}
}

// Feature: inbound filtering
// Scenario: requireDKIM accepts only a cryptographically valid signature whose
// d= matches an allowed domain
func TestEvaluateRequireDKIM(t *testing.T) {
	priv := testKey(t)
	const domain = "email.apple.com"
	const selector = "sel"
	resolver := txtResolver(t, priv, domain, selector)
	filters := &v1alpha1.Filters{RequireDKIM: []string{domain}}

	// No signature at all is rejected.
	without := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 100, resolver)
	if without.Accept || without.Reason != ReasonDKIM {
		t.Errorf("expected dkim rejection without signature, got %+v", without)
	}

	// A real, verifiable signature for the allowed domain is accepted.
	signed := signMessage(t, priv, domain, selector, appleMessage(""))
	with := Evaluate(filters, "noreply@email.apple.com", signed, int64(len(signed)), resolver)
	if !with.Accept {
		t.Errorf("expected accept with valid dkim signature, got %+v", with)
	}

	// A structurally-present but cryptographically invalid signature is rejected.
	forged := "DKIM-Signature: v=1; a=rsa-sha256; d=email.apple.com; s=sel; bh=x; b=y\r\n"
	tampered := Evaluate(filters, "noreply@email.apple.com", appleMessage(forged), 100, resolver)
	if tampered.Accept || tampered.Reason != ReasonDKIM {
		t.Errorf("expected dkim rejection for forged signature, got %+v", tampered)
	}

	// A valid signature whose domain is not allowed is rejected.
	otherKey := testKey(t)
	otherSigned := signMessage(t, otherKey, "evil.example", selector, appleMessage(""))
	otherResolver := txtResolver(t, otherKey, "evil.example", selector)
	wrongDomain := Evaluate(filters, "noreply@email.apple.com", otherSigned, int64(len(otherSigned)), otherResolver)
	if wrongDomain.Accept || wrongDomain.Reason != ReasonDKIM {
		t.Errorf("expected dkim rejection for non-allowed domain, got %+v", wrongDomain)
	}
}

// Feature: inbound scoring
// Scenario: the dkimDomain and authResults signals reflect a cryptographically
// valid signature for an allowed domain
func TestEvaluateDKIMScoreSignals(t *testing.T) {
	priv := testKey(t)
	const domain = "email.apple.com"
	const selector = "sel"
	resolver := txtResolver(t, priv, domain, selector)
	filters := &v1alpha1.Filters{
		AllowedSenderDomains: []string{domain},
		ScoreSignals: []v1alpha1.ScoreSignal{
			v1alpha1.ScoreSignalDKIMDomain,
			v1alpha1.ScoreSignalAuthResults,
		},
		MinScore: 2,
	}

	signed := signMessage(t, priv, domain, selector, appleMessage(""))
	if d := Evaluate(filters, "noreply@email.apple.com", signed, int64(len(signed)), resolver); !d.Accept || d.Score != 2 {
		t.Errorf("expected both dkim signals to score with a valid signature, got %+v", d)
	}

	// Without a valid signature neither signal contributes, so the gate rejects.
	if d := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 100, resolver); d.Accept || d.Reason != ReasonScore {
		t.Errorf("expected score rejection without a valid signature, got %+v", d)
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
	if d := Evaluate(&accept, "noreply@email.apple.com", appleMessage(""), 100, nil); !d.Accept || d.Score != 2 {
		t.Errorf("expected accept with score 2, got %+v", d)
	}

	reject := base
	reject.MinScore = 3
	if d := Evaluate(&reject, "noreply@email.apple.com", appleMessage(""), 100, nil); d.Accept || d.Reason != ReasonScore {
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
	if d := Evaluate(filters, "noreply@email.apple.com", appleMessage(""), 100, nil); !d.Accept || d.Score != 1 {
		t.Errorf("expected body-link signal to score 1 and accept, got %+v", d)
	}
}
