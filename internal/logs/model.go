package logs

import (
	"crypto/sha1"
	"encoding/hex"
	"math"
	"strconv"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// NaT marks a missing or unparseable timestamp (pandas NaT). Comparisons with it are false, as in pandas,
// except where noted.
const NaT int64 = math.MinInt64

// NS is a nullable string (None in Python).
type NS struct {
	S  string
	OK bool
}

// Some returns a present string.
func Some(s string) NS { return NS{s, true} }

// FromValue converts a JSON value: strings stay, null is absent, anything else is its Python str().
func FromValue(v py.Value) NS {
	switch v.K {
	case py.Null:
		return NS{}
	case py.Str:
		return NS{v.S, true}
	}
	return NS{py.PyStr(v), true}
}

// Truthy is Python truthiness of the string (absent or empty is false).
func (n NS) Truthy() bool { return n.OK && n.S != "" }

// Tri is a Python value that is None, False or True (the call "failed" flag).
type Tri uint8

const (
	TriNone Tri = iota
	TriFalse
	TriTrue
)

// TriOf converts a bool.
func TriOf(b bool) Tri {
	if b {
		return TriTrue
	}
	return TriFalse
}

// Thread is one log file: a main conversation or a sub-agent.
type Thread struct {
	Tool, File   string
	ThreadID     string
	SessionID    NS
	ParentID     NS
	Source       NS
	Cwd          NS
	Start, End   int64 // from the earliest and latest timestamp strings (compared as strings, as Python does)
	Model        NS
	Version      NS
	NAborted     int
	NCompactions int
	CostUSD      py.Value // number or null
	LinesAdded   py.Value
	HasFinalMsg  bool
	Originator   NS
	Entrypoint   NS
	// LaunchSource is Codex's session_meta "source" when it is a plain value (cli, vscode, exec, mcp, ...).
	LaunchSource NS
	// TypedPrompts counts Claude user prompts recorded as typed at the keyboard (promptSource typed,
	// suggestion_accepted or queued), which only the interactive terminal UI records.
	TypedPrompts int
	NSteps       int
	NCalls       int
	NHumanTurns  int
	Bytes        int64
}

// Step is one model response.
type Step struct {
	Seq             int
	TS              int64
	Trigger         NS
	Uncached        int64
	Cached          int64
	CacheWrite      int64
	Output          int64
	Reasoning       int64
	InputTotal      int64
	AfterCompaction bool
	// Model is the model recorded on this response (Claude: the message's model; Codex: the turn's model).
	Model NS
	// CW5m and CW1h split CacheWrite by cache lifetime (Claude Code's usage.cache_creation; both 0 when absent).
	CW5m, CW1h int64
}

// Call is one tool call.
type Call struct {
	TS       int64
	CallID   NS
	Name     NS
	Kind     string
	Target   NS
	Path     NS
	Failed   Tri
	OutChars int
	InChars  int // -1: not recorded (NaN in the reference)
	Step     int // -1 while waiting for the response that issued it
	Cwd      NS
	HasCwd   bool
}

// Turn is one user turn (human request, slash command or interrupt).
type Turn struct {
	TS   int64
	Kind string
	Text string
}

// Commit is a commit hash printed by `git commit` in a session.
type Commit struct {
	TS     int64
	SHA    string
	Branch string
	Cwd    NS
}

// Event is something that can precede a cache rewrite (model change, tool list change, new request, ...).
type Event struct {
	TS   int64
	Kind string
}

// File is everything read from one log file.
type File struct {
	Thread  Thread
	Steps   []Step
	Calls   []Call
	Turns   []Turn
	Commits []Commit
	Events  []Event
	Stats   Stats
	Setup   *Setup // nil when the file says nothing about the setup
}

// H is the reference's h(): the first 16 hex digits of the SHA-1 of the UTF-8 text
// (lone surrogates become "?", as encode('utf-8', 'replace') does).
func H(s string) string {
	sum := sha1.Sum([]byte(py.ReplaceSurrogates(s, "?")))
	return hex.EncodeToString(sum[:])[:16]
}

// ParseTime parses an ISO 8601 timestamp as pandas.to_datetime(format='ISO8601', utc=True) does for the
// forms the logs use: date, optional time with fraction, optional Z or offset (no offset means UTC).
// Returns NaT when it cannot be read.
func ParseTime(s string) int64 {
	n := len(s)
	if n < 10 || s[4] != '-' || s[7] != '-' {
		return NaT
	}
	num := func(a, b int) (int, bool) {
		if b > n {
			return 0, false
		}
		v := 0
		for i := a; i < b; i++ {
			c := s[i]
			if c < '0' || c > '9' {
				return 0, false
			}
			v = v*10 + int(c-'0')
		}
		return v, true
	}
	y, ok1 := num(0, 4)
	mo, ok2 := num(5, 7)
	d, ok3 := num(8, 10)
	if !ok1 || !ok2 || !ok3 || mo < 1 || mo > 12 || d < 1 || d > daysIn(y, mo) {
		return NaT
	}
	var hh, mm, ss int
	var frac int64
	i := 10
	if i < n && (s[i] == 'T' || s[i] == ' ') {
		var ok bool
		if hh, ok = num(i+1, i+3); !ok || i+3 >= n+1 {
			return NaT
		}
		i += 3
		if i < n && s[i] == ':' {
			if mm, ok = num(i+1, i+3); !ok {
				return NaT
			}
			i += 3
			if i < n && s[i] == ':' {
				if ss, ok = num(i+1, i+3); !ok {
					return NaT
				}
				i += 3
				if i < n && (s[i] == '.' || s[i] == ',') {
					j := i + 1
					digits := 0
					for j < n && s[j] >= '0' && s[j] <= '9' {
						if digits < 9 {
							frac = frac*10 + int64(s[j]-'0')
						}
						digits++
						j++
					}
					if digits == 0 {
						return NaT
					}
					for k := digits; k < 9; k++ {
						frac *= 10
					}
					i = j
				}
			}
		}
		if hh > 23 || mm > 59 || ss > 59 {
			return NaT
		}
	}
	off := 0
	if i < n {
		switch s[i] {
		case 'Z', 'z':
			i++
		case '+', '-':
			sign := 1
			if s[i] == '-' {
				sign = -1
			}
			oh, ok := num(i+1, i+3)
			if !ok {
				return NaT
			}
			i += 3
			om := 0
			if i < n && s[i] == ':' {
				i++
			}
			if i+2 <= n {
				if v, ok := num(i, i+2); ok {
					om = v
					i += 2
				}
			}
			off = sign * (oh*3600 + om*60)
		}
	}
	if i != n {
		return NaT
	}
	days := daysFromCivil(y, mo, d)
	secs := days*86400 + int64(hh*3600+mm*60+ss) - int64(off)
	return secs*1e9 + frac
}

func daysIn(y, m int) int {
	switch m {
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// daysFromCivil: days since 1970-01-01 (proleptic Gregorian).
func daysFromCivil(y, m, d int) int64 {
	if m <= 2 {
		y--
	}
	era := y / 400
	if y < 0 && y%400 != 0 {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return int64(era)*146097 + int64(doe) - 719468
}

// tsOf converts a raw "timestamp" value.
func tsOf(v py.Value) int64 {
	if v.K != py.Str {
		return NaT
	}
	return ParseTime(v.S)
}

// maxTokens clamps token counts so a corrupt value cannot overflow or turn negative (conversion of an
// out-of-range float to int64 differs between processors).
const maxTokens = 1 << 53

func intOf(v py.Value) int64 {
	f, ok := v.Num()
	if !ok {
		return 0
	}
	switch {
	case f != f: // NaN
		return 0
	case f > maxTokens:
		return maxTokens
	case f < -maxTokens:
		return -maxTokens
	}
	if v.K == py.Int && v.Big == "" {
		return v.I
	}
	return int64(f)
}

// orZero is Python's "d.get(k, 0) or 0" for a number.
func orZero(v py.Value) int64 {
	if !v.Truthy() {
		return 0
	}
	return intOf(v)
}

// Equal is Python == for decoded JSON values (dict order does not matter; 1 == 1.0 == True).
func Equal(a, b py.Value) bool {
	if fa, ok := a.Num(); ok {
		if fb, ok2 := b.Num(); ok2 {
			if a.Big != "" || b.Big != "" {
				return a.K == b.K && a.Big == b.Big
			}
			return fa == fb
		}
		return false
	}
	if a.K != b.K {
		return false
	}
	switch a.K {
	case py.Null:
		return true
	case py.Str:
		return a.S == b.S
	case py.Arr:
		if len(a.A) != len(b.A) {
			return false
		}
		for i := range a.A {
			if !Equal(a.A[i], b.A[i]) {
				return false
			}
		}
		return true
	case py.Obj:
		if len(a.O.Keys) != len(b.O.Keys) {
			return false
		}
		for i, k := range a.O.Keys {
			bv, ok := b.O.Get(k)
			if !ok || !Equal(a.O.Vals[i], bv) {
				return false
			}
		}
		return true
	}
	return false
}

// key is a map key for a Python value used as a dict key (call ids, message ids).
func key(v py.Value) string {
	if v.K == py.Str {
		return "s" + v.S
	}
	if f, ok := v.Num(); ok && v.Big == "" {
		return "n" + strconv.FormatFloat(f, 'g', -1, 64)
	}
	return "r" + py.Repr(v)
}
