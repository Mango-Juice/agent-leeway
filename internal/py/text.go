// Package py reproduces the few Python string, number and JSON behaviors the report depends on, so the Go
// implementation gives the same numbers and text as the Python reference (research/oracle).
//
// Strings are Go strings holding UTF-8. JSON "\ud800"-style escapes that are not part of a surrogate pair
// cannot be UTF-8; they are kept as 3-byte WTF-8 sequences (ED A0..BF xx) so that lengths, hashes and
// re-serialization match Python, where such a string holds a lone surrogate code point.
// All lengths and slices here count code points, as Python's len() does, never bytes.
package py

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// DecodeRune decodes one code point, accepting WTF-8 lone surrogates. Invalid bytes decode as
// (utf8.RuneError, 1); input from log files is sanitized with DecodeBytes first, so that does not occur.
func DecodeRune(s string) (rune, int) {
	if len(s) >= 3 && s[0] == 0xED && s[1] >= 0xA0 && s[1] <= 0xBF && s[2] >= 0x80 && s[2] <= 0xBF {
		return rune(0xD000) | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	}
	return utf8.DecodeRuneInString(s)
}

// AppendRune appends r as UTF-8, or as WTF-8 when r is a surrogate.
func AppendRune(b []byte, r rune) []byte {
	if r >= 0xD800 && r <= 0xDFFF {
		return append(b, 0xED, byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	}
	return utf8.AppendRune(b, r)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// Len is Python's len(str): the number of code points.
func Len(s string) int {
	if isASCII(s) {
		return len(s)
	}
	n := utf8.RuneCountInString(s)
	if strings.IndexByte(s, 0xED) >= 0 { // a WTF-8 surrogate counts as 3 invalid runes above
		for i := 0; i < len(s); {
			_, sz := DecodeRune(s[i:])
			if sz == 3 && s[i] == 0xED && s[i+1] >= 0xA0 {
				n -= 2
			}
			i += sz
		}
	}
	return n
}

// Runes returns the code points of s (surrogates included).
func Runes(s string) []rune {
	out := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		r, sz := DecodeRune(s[i:])
		out = append(out, r)
		i += sz
	}
	return out
}

// byteOffset returns the byte index of code point n (clamped to the string).
func byteOffset(s string, n int) int {
	if n <= 0 {
		return 0
	}
	i := 0
	for k := 0; k < n && i < len(s); k++ {
		_, sz := DecodeRune(s[i:])
		i += sz
	}
	return i
}

// Head is s[:n] in Python (code points), n >= 0.
func Head(s string, n int) string {
	if isASCII(s) {
		if n >= len(s) {
			return s
		}
		return s[:n]
	}
	return s[:byteOffset(s, n)]
}

// Tail is s[-n:] in Python (code points), n >= 0; n == 0 gives the whole string, as in Python.
func Tail(s string, n int) string {
	if n == 0 {
		return s
	}
	l := Len(s)
	if n >= l {
		return s
	}
	if isASCII(s) {
		return s[l-n:]
	}
	return s[byteOffset(s, l-n):]
}

// IsSpace is Python's str.isspace() for one code point (also what re's \s matches).
func IsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1C && r <= 0x1F)
}

// IsWord is what Python's re \w matches for one code point (letters, numbers, underscore).
func IsWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// Strip is Python's str.strip() with no argument.
func Strip(s string) string { return RStrip(LStrip(s)) }

// LStrip is Python's str.lstrip() with no argument.
func LStrip(s string) string {
	i := 0
	for i < len(s) {
		r, sz := DecodeRune(s[i:])
		if !IsSpace(r) {
			break
		}
		i += sz
	}
	return s[i:]
}

// RStrip is Python's str.rstrip() with no argument.
func RStrip(s string) string {
	j := len(s)
	for j > 0 {
		r, sz := utf8.DecodeLastRuneInString(s[:j])
		if !IsSpace(r) {
			break
		}
		j -= sz
	}
	return s[:j]
}

// StripChars is Python's str.strip(chars).
func StripChars(s, chars string) string {
	in := func(r rune) bool { return strings.ContainsRune(chars, r) }
	i := 0
	for i < len(s) {
		r, sz := DecodeRune(s[i:])
		if !in(r) {
			break
		}
		i += sz
	}
	j := len(s)
	for j > i {
		r, sz := utf8.DecodeLastRuneInString(s[i:j])
		if !in(r) {
			break
		}
		j -= sz
	}
	return s[i:j]
}

// Fields is Python's str.split() with no argument.
func Fields(s string) []string {
	var out []string
	start := -1
	for i := 0; i < len(s); {
		r, sz := DecodeRune(s[i:])
		if IsSpace(r) {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
		i += sz
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// CollapseSpace is re.sub(r'\s+', ' ', s) with Python's Unicode \s.
func CollapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inWS := false
	for i := 0; i < len(s); {
		r, sz := DecodeRune(s[i:])
		if IsSpace(r) {
			if !inWS {
				b.WriteByte(' ')
			}
			inWS = true
		} else {
			b.WriteString(s[i : i+sz])
			inWS = false
		}
		i += sz
	}
	return b.String()
}

// DigitValue is the value of a Unicode decimal digit (category Nd), as Python's int() reads it.
func DigitValue(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	k := 0
	for unicode.Is(unicode.Nd, r-rune(k)-1) {
		k++
	}
	return k % 10
}

// ParseInt is Python's int() for a string of an optional sign and Unicode decimal digits. ok is false when
// the value does not fit in int64 (callers only compare it with 0).
func ParseInt(s string) (v int64, nonzero bool, ok bool) {
	neg := false
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	}
	ok = true
	for _, r := range s {
		d := int64(DigitValue(r))
		if d != 0 {
			nonzero = true
		}
		if v > (1<<62)/10 {
			ok = false
			continue
		}
		v = v*10 + d
	}
	if neg {
		v = -v
	}
	return v, nonzero, ok
}

// ToUpper is str.upper() for the case-insensitive project filter (simple case mapping; Python's special
// cases such as "ß" -> "SS" are not reproduced).
func ToUpper(s string) string { return strings.ToUpper(s) }

// DecodeBytes decodes UTF-8 as Python's bytes.decode('utf-8', 'replace') does: each maximal invalid
// subsequence becomes one U+FFFD (the Unicode "maximal subpart" practice that CPython follows).
func DecodeBytes(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	out := make([]byte, 0, len(b)+16)
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			out = append(out, c)
			i++
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || sz > 1 {
			out = append(out, b[i:i+sz]...)
			i += sz
			continue
		}
		// invalid: consume the longest prefix that could still start a valid sequence
		n := maximalSubpart(b[i:])
		out = append(out, "�"...)
		i += n
	}
	return string(out)
}

func maximalSubpart(b []byte) int {
	c := b[0]
	var need int
	lo, hi := byte(0x80), byte(0xBF)
	switch {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c == 0xE0:
		need, lo = 2, 0xA0
	case c >= 0xE1 && c <= 0xEC, c == 0xEE, c == 0xEF:
		need = 2
	case c == 0xED:
		need, hi = 2, 0x9F
	case c == 0xF0:
		need, lo = 3, 0x90
	case c >= 0xF1 && c <= 0xF3:
		need = 3
	case c == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	n := 1
	for k := 0; k < need && n < len(b); k++ {
		x := b[n]
		if k == 0 {
			if x < lo || x > hi {
				return n
			}
		} else if x < 0x80 || x > 0xBF {
			return n
		}
		n++
	}
	return n
}

// ReplaceSurrogates turns WTF-8 lone surrogates into replacement text, for output to terminals and files
// (Python would fail or escape there; we print U+FFFD) or "?" for hashing (Python's encode('utf-8', 'replace')).
func ReplaceSurrogates(s, with string) string {
	if strings.IndexByte(s, 0xED) < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, sz := DecodeRune(s[i:])
		if r >= 0xD800 && r <= 0xDFFF && sz == 3 {
			b.WriteString(with)
		} else {
			b.WriteString(s[i : i+sz])
		}
		i += sz
	}
	return b.String()
}
