/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package relay

import (
	"bytes"
	"net/mail"
	"regexp"
	"strings"

	"github.com/emersion/go-msgauth/dkim"

	"github.com/philprime/iris/api/v1alpha1"
)

// DKIMResolver looks up DKIM public-key TXT records for a domain. Production
// uses net.LookupTXT; tests inject a resolver that returns locally generated
// keys so verification never touches DNS.
type DKIMResolver func(domain string) ([]string, error)

// Rejection reasons. They map to SMTP responses and the iris_relay_messages_total
// result label.
const (
	ReasonSize   = "size"
	ReasonSender = "sender"
	ReasonDKIM   = "dkim"
	ReasonScore  = "score"
)

// Decision is the outcome of evaluating a message against a relay's filters.
type Decision struct {
	// Accept reports whether the message passes all hard rules and the score gate.
	Accept bool
	// Reason names the failed check when Accept is false.
	Reason string
	// Score is the heuristic score that was computed.
	Score int
}

// Evaluate applies a relay's filters to a message: the hard rules (size, sender
// domain, DKIM) reject first, then the heuristic score is gated against
// minScore. A nil filter set accepts everything.
//
// DKIM is verified cryptographically: requireDKIM and the dkimDomain/authResults
// score signals reflect a valid signature whose d= matches an allowed domain.
// The resolver fetches DKIM public keys (net.LookupTXT in production, an
// injected lookup in tests).
func Evaluate(filters *v1alpha1.Filters, mailFrom string, raw []byte, size int64, resolver DKIMResolver) Decision {
	if filters == nil {
		return Decision{Accept: true}
	}

	if filters.MaxMessageBytes > 0 && size > filters.MaxMessageBytes {
		return Decision{Reason: ReasonSize}
	}

	if len(filters.AllowedSenderDomains) > 0 && !domainMatches(senderDomain(mailFrom), filters.AllowedSenderDomains) {
		return Decision{Reason: ReasonSender}
	}

	header := parseHeader(raw)

	// Verify DKIM only when a rule or signal needs it, since it parses and
	// hashes the whole message and may resolve DNS.
	var verified []string
	if dkimNeeded(filters) {
		verified = verifiedDKIMDomains(raw, resolver)
	}

	if len(filters.RequireDKIM) > 0 && !anyDomainMatches(verified, filters.RequireDKIM) {
		return Decision{Reason: ReasonDKIM}
	}

	score := scoreMessage(filters, header, raw, verified)
	if int(filters.MinScore) > 0 && score < int(filters.MinScore) {
		return Decision{Reason: ReasonScore, Score: score}
	}

	return Decision{Accept: true, Score: score}
}

// dkimNeeded reports whether evaluating the filters requires DKIM verification.
func dkimNeeded(filters *v1alpha1.Filters) bool {
	if len(filters.RequireDKIM) > 0 {
		return true
	}
	for _, signal := range filters.ScoreSignals {
		if signal == v1alpha1.ScoreSignalDKIMDomain || signal == v1alpha1.ScoreSignalAuthResults {
			return true
		}
	}
	return false
}

// verifiedDKIMDomains returns the d= domains of every cryptographically valid
// DKIM signature on the message. A resolver of nil falls back to DNS.
func verifiedDKIMDomains(raw []byte, resolver DKIMResolver) []string {
	opts := &dkim.VerifyOptions{}
	if resolver != nil {
		opts.LookupTXT = resolver
	}
	verifications, err := dkim.VerifyWithOptions(bytes.NewReader(raw), opts)
	if err != nil {
		return nil
	}
	var domains []string
	for _, v := range verifications {
		if v.Err == nil && v.Domain != "" {
			domains = append(domains, strings.ToLower(v.Domain))
		}
	}
	return domains
}

// SenderAllowed reports whether the envelope sender passes the allowed-sender
// hard rule, for an early reject at MAIL FROM time.
func SenderAllowed(filters *v1alpha1.Filters, mailFrom string) bool {
	if filters == nil || len(filters.AllowedSenderDomains) == 0 {
		return true
	}
	return domainMatches(senderDomain(mailFrom), filters.AllowedSenderDomains)
}

func scoreMessage(filters *v1alpha1.Filters, header mail.Header, raw []byte, verified []string) int {
	allowed := filters.AllowedSenderDomains
	score := 0
	for _, signal := range filters.ScoreSignals {
		if signalMatches(signal, header, raw, allowed, verified) {
			score++
		}
	}
	return score
}

func signalMatches(signal v1alpha1.ScoreSignal, header mail.Header, raw []byte, allowed, verified []string) bool {
	switch signal {
	case v1alpha1.ScoreSignalFromDomain:
		return domainMatches(addressDomain(header.Get("From")), allowed)
	case v1alpha1.ScoreSignalMessageIDDomain:
		return domainMatches(messageIDDomain(header.Get("Message-ID")), allowed)
	case v1alpha1.ScoreSignalDKIMDomain, v1alpha1.ScoreSignalAuthResults:
		// Both signals now require a cryptographically valid signature whose
		// d= matches an allowed domain.
		return anyDomainMatches(verified, allowed)
	case v1alpha1.ScoreSignalBodyLinkDomain:
		return bodyLinksToAllowed(raw, allowed)
	default:
		return false
	}
}

func parseHeader(raw []byte) mail.Header {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return mail.Header{}
	}
	return msg.Header
}

func senderDomain(addr string) string {
	if at := strings.LastIndexByte(addr, '@'); at >= 0 {
		return strings.ToLower(strings.Trim(addr[at+1:], "> "))
	}
	return ""
}

func addressDomain(headerValue string) string {
	if parsed, err := mail.ParseAddress(headerValue); err == nil {
		return senderDomain(parsed.Address)
	}
	return senderDomain(headerValue)
}

func messageIDDomain(messageID string) string {
	return senderDomain(strings.Trim(messageID, "<> "))
}

// anyDomainMatches reports whether any host in hosts equals or is a subdomain
// of any allowed domain.
func anyDomainMatches(hosts, allowed []string) bool {
	for _, host := range hosts {
		if domainMatches(host, allowed) {
			return true
		}
	}
	return false
}

var urlPattern = regexp.MustCompile(`https?://([^/\s"'>]+)`)

func bodyLinksToAllowed(raw []byte, allowed []string) bool {
	body := messageBody(raw)
	for _, match := range urlPattern.FindAllStringSubmatch(body, -1) {
		if domainMatches(strings.ToLower(match[1]), allowed) {
			return true
		}
	}
	return false
}

func messageBody(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return string(raw)
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(msg.Body)
	return buf.String()
}

// domainMatches reports whether host equals or is a subdomain of any allowed
// domain.
func domainMatches(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, domain := range allowed {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
