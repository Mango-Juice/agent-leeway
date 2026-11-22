package py

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Format is Python's str.format(**args) for the message templates: "{name}" and "{name:spec}" with the
// format-spec mini language ([[fill]align][sign][0][width][,|_][.precision][type]; types d, f, %, s or none),
// and "{{" / "}}" escapes. Values may be int, int64, float64, string, bool or Value.
func Format(tmpl string, args map[string]any) string {
	var b strings.Builder
	for i := 0; i < len(tmpl); {
		c := tmpl[i]
		if c == '{' {
			if i+1 < len(tmpl) && tmpl[i+1] == '{' {
				b.WriteByte('{')
				i += 2
				continue
			}
			j := strings.IndexByte(tmpl[i:], '}')
			if j < 0 {
				panic("format: unclosed field in " + tmpl)
			}
			field := tmpl[i+1 : i+j]
			name, spec, _ := strings.Cut(field, ":")
			v, ok := args[name]
			if !ok {
				panic("format: missing argument " + name + " for " + tmpl)
			}
			b.WriteString(FormatValue(v, spec))
			i += j + 1
			continue
		}
		if c == '}' && i+1 < len(tmpl) && tmpl[i+1] == '}' {
			b.WriteByte('}')
			i += 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

type spec struct {
	fill     rune
	align    byte
	sign     byte
	zero     bool
	width    int
	group    byte
	prec     int
	typ      byte
	hasAlign bool
}

func parseSpec(s string) spec {
	sp := spec{fill: ' ', prec: -1}
	r := []rune(s)
	i := 0
	isAlign := func(c rune) bool { return c == '<' || c == '>' || c == '=' || c == '^' }
	if len(r) >= 2 && isAlign(r[1]) {
		sp.fill, sp.align, sp.hasAlign = r[0], byte(r[1]), true
		i = 2
	} else if len(r) >= 1 && isAlign(r[0]) {
		sp.align, sp.hasAlign = byte(r[0]), true
		i = 1
	}
	if i < len(r) && (r[i] == '+' || r[i] == '-' || r[i] == ' ') {
		sp.sign = byte(r[i])
		i++
	}
	if i < len(r) && r[i] == '#' {
		i++
	}
	if i < len(r) && r[i] == '0' {
		sp.zero = true
		i++
	}
	st := i
	for i < len(r) && r[i] >= '0' && r[i] <= '9' {
		i++
	}
	if i > st {
		sp.width, _ = strconv.Atoi(string(r[st:i]))
	}
	if i < len(r) && (r[i] == ',' || r[i] == '_') {
		sp.group = byte(r[i])
		i++
	}
	if i < len(r) && r[i] == '.' {
		i++
		st = i
		for i < len(r) && r[i] >= '0' && r[i] <= '9' {
			i++
		}
		sp.prec, _ = strconv.Atoi(string(r[st:i]))
	}
	if i < len(r) {
		sp.typ = byte(r[i])
	}
	return sp
}

// FormatValue is format(v, spec).
func FormatValue(v any, specText string) string {
	sp := parseSpec(specText)
	switch x := v.(type) {
	case Value:
		switch x.K {
		case Int:
			if x.Big == "" {
				return FormatValue(x.I, specText)
			}
		case Float:
			return FormatValue(x.F, specText)
		case Str:
			return FormatValue(x.S, specText)
		case Bool:
			return FormatValue(x.B, specText)
		}
		return pad(PyStr(x), sp, '<')
	case string:
		s := x
		if sp.prec >= 0 {
			s = Head(s, sp.prec)
		}
		return pad(s, sp, '<')
	case bool:
		if specText == "" {
			if x {
				return "True"
			}
			return "False"
		}
		if x {
			return FormatValue(int64(1), specText)
		}
		return FormatValue(int64(0), specText)
	case int:
		return FormatValue(int64(x), specText)
	case int64:
		if sp.typ == 'f' || sp.typ == '%' || sp.typ == 'e' || sp.typ == 'g' {
			return FormatValue(float64(x), specText)
		}
		neg := x < 0
		u := x
		if neg {
			u = -x
		}
		digits := group(strconv.FormatInt(u, 10), sp.group)
		return number(digits, neg, sp)
	case float64:
		var body string
		neg := math.Signbit(x) && !math.IsNaN(x)
		a := math.Abs(x)
		switch sp.typ {
		case 'f', 'F':
			p := sp.prec
			if p < 0 {
				p = 6
			}
			body = fixed(a, p, sp.group)
		case '%':
			p := sp.prec
			if p < 0 {
				p = 6
			}
			body = fixed(a*100, p, sp.group) + "%"
		case 0:
			if sp.prec >= 0 {
				panic("format: general float format with precision is not used")
			}
			body = FloatRepr(a, false)
			if sp.group != 0 {
				ip, fp, _ := strings.Cut(body, ".")
				body = group(ip, sp.group) + "." + fp
			}
		default:
			panic(fmt.Sprintf("format: float type %q not supported", sp.typ))
		}
		return number(body, neg, sp)
	}
	panic(fmt.Sprintf("format: unsupported value %T", v))
}

func fixed(a float64, prec int, g byte) string {
	if math.IsInf(a, 0) {
		return "inf"
	}
	if math.IsNaN(a) {
		return "nan"
	}
	s := strconv.FormatFloat(a, 'f', prec, 64)
	if g == 0 {
		return s
	}
	ip, fp, has := strings.Cut(s, ".")
	if has {
		return group(ip, g) + "." + fp
	}
	return group(ip, g)
}

func group(digits string, g byte) string {
	if g == 0 || len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	first := len(digits) % 3
	if first > 0 {
		b.WriteString(digits[:first])
	}
	for i := first; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(g)
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func number(body string, neg bool, sp spec) string {
	sign := ""
	if neg {
		sign = "-"
	} else if sp.sign == '+' {
		sign = "+"
	} else if sp.sign == ' ' {
		sign = " "
	}
	if sp.zero && !sp.hasAlign {
		sp.fill, sp.align, sp.hasAlign = '0', '=', true
	}
	if sp.align == '=' {
		n := sp.width - Len(sign) - Len(body)
		if n > 0 {
			return sign + strings.Repeat(string(sp.fill), n) + body
		}
		return sign + body
	}
	return pad(sign+body, sp, '>')
}

func pad(s string, sp spec, def byte) string {
	n := sp.width - Len(s)
	if n <= 0 {
		return s
	}
	align := sp.align
	if !sp.hasAlign {
		align = def
	}
	f := string(sp.fill)
	switch align {
	case '<':
		return s + strings.Repeat(f, n)
	case '^':
		l := n / 2
		return strings.Repeat(f, l) + s + strings.Repeat(f, n-l)
	default:
		return strings.Repeat(f, n) + s
	}
}

// Round is Python's round(x, n) for floats (correctly rounded, ties to even on the exact binary value).
func Round(x float64, n int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	f, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', n, 64), 64)
	return f
}

// RoundNumpy is numpy.round(x, n) (what pandas DataFrame.round uses): rint(x * 10**n) / 10**n,
// which can differ from Python's round() in the last digit.
func RoundNumpy(x float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.RoundToEven(x*p) / p
}
