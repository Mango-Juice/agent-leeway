// Package secrets masks credentials in text shown to people (report excerpts, summaries, terminal lines).
//
// The rules follow the maintained gitleaks default rule set
// (https://github.com/gitleaks/gitleaks/blob/master/config/gitleaks.toml, read 2026-11-22), adapted:
//   - provider tokens keep the gitleaks prefixes but accept a shorter tail (20+ characters), because a judge's
//     evidence quote can stop in the middle of a token;
//   - the generic rule (gitleaks generic-api-key) is a key-like name, then = or :, then a value of 16+
//     characters whose Shannon entropy is at least 3.5 bits per character (the gitleaks threshold);
//   - bearer and basic credentials in Authorization headers (gitleaks curl-auth-header, without requiring curl).
//
// Only display copies are masked. The judge input, the stored results and the citation check keep the original
// text, so a masked excerpt still points to a quote that is in the transcript.
package secrets

import (
	"math"
	"regexp"
	"strings"
)

// Rule is one secret pattern; Group is the submatch holding the secret (0 = the whole match).
type Rule struct {
	ID    string
	Re    *regexp.Regexp
	Group int
}

// Rules are applied in order.
var Rules = []Rule{
	{"private-key", regexp.MustCompile(`(?is)-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----.*?(?:-----END[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----|$)`), 0},
	{"anthropic-api-key", regexp.MustCompile(`sk-ant-(?:api\d\d|admin\d\d)-[a-zA-Z0-9_\-]{20,}`), 0},
	{"openai-api-key", regexp.MustCompile(`sk-(?:proj|svcacct|admin)-[A-Za-z0-9_\-]{20,}|\bsk-[A-Za-z0-9]{20,}T3BlbkFJ[A-Za-z0-9]*|\bsk-[A-Za-z0-9]{40,}`), 0},
	{"aws-access-token", regexp.MustCompile(`\b(?:A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16}\b`), 0},
	{"github-token", regexp.MustCompile(`\b(?:gh[pousr]_[0-9a-zA-Z]{20,}|github_pat_\w{20,})`), 0},
	{"gitlab-pat", regexp.MustCompile(`\bglpat-[\w-]{20,}`), 0},
	{"slack-token", regexp.MustCompile(`\bxox[abeoprs]-[0-9A-Za-z-]{10,}`), 0},
	{"stripe-access-token", regexp.MustCompile(`\b(?:sk|rk)_(?:test|live|prod)_[a-zA-Z0-9]{10,99}`), 0},
	{"gcp-api-key", regexp.MustCompile(`\bAIza[\w-]{30,}`), 0},
	{"jwt", regexp.MustCompile(`\bey[a-zA-Z0-9]{17,}\.ey[a-zA-Z0-9/\\_-]{17,}\.(?:[a-zA-Z0-9/\\_-]{10,}={0,2})?`), 0},
	{"authorization-header", regexp.MustCompile(`(?i)\b(?:bearer|basic|token)\s+([A-Za-z0-9._~+/=-]{16,})`), 1},
	{"url-password", regexp.MustCompile(`://[^/\s:@]+:([^/\s@]{3,})@`), 1},
	{"generic-api-key", regexp.MustCompile(`(?i)\b[\w.-]{0,30}(?:access|api|auth|key|credential|creds|passwd|password|secret|token)[\w.-]{0,30}["']?\s*(?:=|:|=>|:=)\s*["']?([A-Za-z0-9+/=_.~!@#$%^&*-]{16,})`), 1},
}

// entropy is the Shannon entropy in bits per character.
func entropy(s string) float64 {
	n := map[rune]float64{}
	total := 0.0
	for _, r := range s {
		n[r]++
		total++
	}
	h := 0.0
	for _, c := range n {
		p := c / total
		h -= p * math.Log2(p)
	}
	return h
}

// Masked is Mask, plus whether anything was masked (the report counts masked quotes).
func Masked(s string) (string, bool) {
	m := Mask(s)
	return m, m != s
}

// Mask replaces every secret in s with "[masked <rule>]". Commit hashes and other bare hex strings are kept.
func Mask(s string) string {
	if s == "" {
		return s
	}
	for _, r := range Rules {
		s = maskRule(s, r)
	}
	return s
}

func maskRule(s string, r Rule) string {
	locs := r.Re.FindAllStringSubmatchIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range locs {
		a, e := m[2*r.Group], m[2*r.Group+1]
		if a < 0 {
			continue
		}
		val := s[a:e]
		if r.ID == "generic-api-key" && (entropy(val) < 3.5 || strings.HasPrefix(val, "[masked")) {
			continue
		}
		if r.ID == "authorization-header" && entropy(val) < 3.0 {
			continue
		}
		b.WriteString(s[last:a])
		b.WriteString("[masked " + r.ID + "]")
		last = e
	}
	b.WriteString(s[last:])
	return b.String()
}
