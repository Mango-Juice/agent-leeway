package messages

import (
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// fromReference maps texts the Python reference printed to the Go product's wording. Parity tests apply it to
// the reference's outputs, so everything else must still match byte for byte.
var fromReference = [][2]string{
	{"Session keys are in the --json output.", "Session keys are in the --json output. Tags: [automation] = most of the session's leak came from automation runs; [direct+automation N%] = N% of it did."},
	{"By habit: per $100 of spend (before -> after)", "By habit, all sessions (direct and automation): per $100 of spend (before -> after)"},
	{"Privacy: logs, cache and reports stay on this computer. The only thing sent out is the drift check: each selected session goes to its own provider's official CLI (claude -p or codex exec) for judging.", "Privacy: logs, cache and reports stay on this computer. Two things go out: the drift check sends each selected session to its own provider's official CLI (claude -p or codex exec), which sends it wherever that CLI is set up to (its logged-in account, or a base URL or proxy you configured) together with the CLI's own telemetry; and `self-update --check` asks GitHub for the latest release tag. Nothing else opens a network connection (`privacy` lists the details)."},
	{"(Claude 1 h, Codex ~10 min assumed)", "(Claude Code: 5 minutes or 1 hour, as each cache write records; Codex: 30 minutes on GPT-5.6 and later)"},
	{"After a long gap (Claude over 1 h, Codex over ~10 min), leave a summary and start a new session.", "After a gap longer than the cache lifetime (Claude Code: 5 min or 1 h, Codex: 30 min), leave a summary and start a new session."},
	{"Amounts are USD at API list prices (a subscription is not billed this way; read them as list-price equivalents). Codex model prices are not public: GPT-5 list prices are assumed.", "Amounts are USD at the providers' API list prices (price table read {read}; `agent-leeway prices` shows it). A subscription is not billed per token: read them as list-price equivalents."},
	{" (USD at API list prices, drawn from your subscription usage)", " at API list prices, billed to whatever account the CLI is logged in with (plan quota on a subscription, money on an API key)"},
}

// FromReference rewrites a reference output into the Go wording (parity tests only). read is the price
// table date that fills {read}.
func FromReference(s, read string) string {
	for _, r := range fromReference {
		s = strings.ReplaceAll(s, r[0], strings.ReplaceAll(r[1], "{read}", read))
	}
	return s
}

// Money is MoneyNote with the price table date.
func Money(read string) string { return strings.ReplaceAll(MoneyNote, "{read}", read) }

// Times is "1 time" or "N times".
func Times(n int) string {
	if n == 1 {
		return "1 time"
	}
	return py.Format("{n:,} times", map[string]any{"n": n})
}

// FromReferenceHTML is FromReference for HTML output (both sides escaped as the report escapes them).
func FromReferenceHTML(s, read string, escape func(string) string) string {
	for _, r := range fromReference {
		s = strings.ReplaceAll(s, escape(r[0]), escape(strings.ReplaceAll(r[1], "{read}", read)))
	}
	return s
}
