package py

import (
	"regexp"
	"strings"
)

// wsClass is the content of a character class matching what Python's re \s matches in str patterns.
const wsClass = `\t-\r\x{1c}-\x{20}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}`

// Re compiles a Python-syntax pattern with Python's Unicode meanings of \s, \S and \d
// (Go's RE2 classes are ASCII-only). \b, lookaround and backreferences are not translated:
// callers that need them implement that part by hand.
func Re(pattern string) *regexp.Regexp {
	var b strings.Builder
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			n := pattern[i+1]
			switch {
			case n == 's' && !inClass:
				b.WriteString("[" + wsClass + "]")
			case n == 's' && inClass:
				b.WriteString(wsClass)
			case n == 'S' && !inClass:
				b.WriteString("[^" + wsClass + "]")
			case n == 'd':
				b.WriteString(`\p{Nd}`)
			case n == 'b':
				panic("py.Re: \\b is not translated: " + pattern)
			default:
				b.WriteByte(c)
				b.WriteByte(n)
			}
			i++
			continue
		}
		if c == '[' && !inClass {
			inClass = true
			b.WriteByte(c)
			if i+1 < len(pattern) && pattern[i+1] == '^' {
				b.WriteByte('^')
				i++
			}
			if i+1 < len(pattern) && pattern[i+1] == ']' {
				b.WriteString(`\]`)
				i++
			}
			continue
		}
		if c == ']' && inClass {
			inClass = false
		}
		b.WriteByte(c)
	}
	return regexp.MustCompile(b.String())
}
