package secrets

import (
	"strings"
	"testing"
)

// fake secrets only (none of these are real credentials)
var fakes = map[string]string{
	"anthropic-api-key":    "sk-ant-api03-" + strings.Repeat("aB3dE5fG7h", 4),
	"openai-api-key":       "sk-proj-" + strings.Repeat("Zx9Yw8Vu7T", 4),
	"aws-access-token":     "AKIA" + "Q3J5K7L2M4N6P7R2",
	"github-token":         "ghp_" + strings.Repeat("a1B2c3D4e5", 4),
	"gitlab-pat":           "glpat-" + "x1Y2z3W4v5U6t7S8r9Q0",
	"slack-token":          "xo" + "xb-" + "123456789012-abcdefGHIJ",
	"stripe-access-token":  "sk_live_" + "51Habcdefghijk",
	"gcp-api-key":          "AIza" + "SyA1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6",
	"jwt":                  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
	"authorization-header": "Bearer " + "q8Wf3Lx0Tz7Rk2Vn5Ys9",
	"generic-api-key":      "api_key = \"Hq7#Lm2pXv9!Rt4wZs8b\"",
	"private-key":          "-----BEGIN OPENSSH " + "PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAE\n-----END OPENSSH PRIVATE KEY-----",
}

func TestMaskProviderSecretsInKoreanText(t *testing.T) {
	for id, secret := range fakes {
		text := "여기에 키를 붙여 넣었어요: " + secret + " 이걸로 배포해 줘."
		got := Mask(text)
		if strings.Contains(got, strings.TrimPrefix(strings.TrimPrefix(secret, "Bearer "), "api_key = \"")[:12]) {
			t.Errorf("%s not masked: %s", id, got)
		}
		if !strings.Contains(got, "[masked") || !strings.HasPrefix(got, "여기에 키를 붙여 넣었어요:") || !strings.HasSuffix(got, "이걸로 배포해 줘.") {
			t.Errorf("%s: surrounding text changed: %s", id, got)
		}
	}
}

func TestOrdinaryTextIsKept(t *testing.T) {
	for _, s := range []string{
		"표는 그대로 두라고 했잖아", "the token limit is 160000 characters", "password: hunter2", "key = value_one_two_three",
		"No, I said keep the table.", "sk-short", "api_key = aaaaaaaaaaaaaaaaaaaa",
	} {
		if got := Mask(s); got != s {
			t.Errorf("%q changed to %q", s, got)
		}
	}
}

func TestURLPasswordAndHashes(t *testing.T) {
	got, ok := Masked("clone https://bob:" + "s3cr3tPw@example.com/repo.git now")
	if !ok || strings.Contains(got, "s3cr3tPw") || !strings.Contains(got, "https://bob:[masked url-password]@example.com") {
		t.Errorf("url password: %q %v", got, ok)
	}
	for _, s := range []string{"commit 3f786850e387550fdab836ed7e6dc881de23001b passed", "see https://example.com/a@b"} {
		if got, ok := Masked(s); ok {
			t.Errorf("%q changed to %q", s, got)
		}
	}
}
