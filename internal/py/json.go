package py

import (
	"errors"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Kind of a JSON value, following Python's json module types (int and float are distinct).
type Kind uint8

const (
	Null Kind = iota
	Bool
	Int
	Float
	Str
	Arr
	Obj
)

// Value is a decoded JSON value. Objects keep key order (Python dicts do); a repeated key keeps its first
// position and takes the last value, as json.loads does.
type Value struct {
	K   Kind
	B   bool
	I   int64  // Int that fits in int64
	Big string // Int that does not fit (decimal digits)
	F   float64
	S   string
	A   []Value
	O   *Object
}

// Object is an ordered JSON object.
type Object struct {
	Keys []string
	Vals []Value
}

// Get returns the value for key (Null and false when absent).
func (o *Object) Get(key string) (Value, bool) {
	if o == nil {
		return Value{}, false
	}
	for i, k := range o.Keys {
		if k == key {
			return o.Vals[i], true
		}
	}
	return Value{}, false
}

func (o *Object) set(key string, v Value) {
	for i, k := range o.Keys {
		if k == key {
			o.Vals[i] = v
			return
		}
	}
	o.Keys = append(o.Keys, key)
	o.Vals = append(o.Vals, v)
}

// Set adds or replaces a key (keeping its first position).
func (o *Object) Set(key string, v Value) { o.set(key, v) }

// Get returns v[key] when v is an object, else Null ("o.get(key)" on a dict).
func (v Value) Get(key string) Value {
	if v.K != Obj {
		return Value{}
	}
	x, _ := v.O.Get(key)
	return x
}

// Has reports whether v is an object holding key.
func (v Value) Has(key string) bool {
	if v.K != Obj {
		return false
	}
	_, ok := v.O.Get(key)
	return ok
}

// IsNull etc. are small helpers.
func (v Value) IsNull() bool { return v.K == Null }
func (v Value) IsStr() bool  { return v.K == Str }
func (v Value) IsObj() bool  { return v.K == Obj }
func (v Value) IsArr() bool  { return v.K == Arr }

// Truthy is Python's bool(v).
func (v Value) Truthy() bool {
	switch v.K {
	case Null:
		return false
	case Bool:
		return v.B
	case Int:
		return v.I != 0 || v.Big != ""
	case Float:
		return v.F != 0
	case Str:
		return v.S != ""
	case Arr:
		return len(v.A) > 0
	case Obj:
		return v.O != nil && len(v.O.Keys) > 0
	}
	return false
}

// Or is Python's "a or b".
func (v Value) Or(b Value) Value {
	if v.Truthy() {
		return v
	}
	return b
}

// StrOr returns the string if v is a string, else def.
func (v Value) StrOr(def string) string {
	if v.K == Str {
		return v.S
	}
	return def
}

// Num returns v as a float64 for numeric kinds (bools count as 0/1 like Python).
func (v Value) Num() (float64, bool) {
	switch v.K {
	case Int:
		if v.Big != "" {
			f, _ := strconv.ParseFloat(v.Big, 64)
			return f, true
		}
		return float64(v.I), true
	case Float:
		return v.F, true
	case Bool:
		if v.B {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// NumEq0 reports whether v == 0 in Python (0, 0.0, False).
func (v Value) NumEq0() bool {
	f, ok := v.Num()
	return ok && f == 0 && v.Big == ""
}

// String builds a string Value.
func String(s string) Value { return Value{K: Str, S: s} }

// ---------------------------------------------------------------- parsing (json.loads)

var errSyntax = errors.New("json: syntax error")

// MaxDepth approximates Python's recursion limit for nested containers.
const MaxDepth = 900

// Loads parses one JSON document the way Python's json.loads(str) does (strict: control characters inside
// strings are errors; NaN, Infinity and -Infinity are accepted; surrounding whitespace is allowed).
func Loads(s string) (Value, error) {
	p := parser{s: s}
	p.ws()
	v, err := p.value(0)
	if err != nil {
		return Value{}, err
	}
	p.ws()
	if p.i != len(p.s) {
		return Value{}, errSyntax
	}
	return v, nil
}

type parser struct {
	s string
	i int
}

func (p *parser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *parser) value(depth int) (Value, error) {
	if depth > MaxDepth {
		return Value{}, errSyntax
	}
	if p.i >= len(p.s) {
		return Value{}, errSyntax
	}
	switch c := p.s[p.i]; {
	case c == '{':
		p.i++
		o := &Object{}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return Value{K: Obj, O: o}, nil
		}
		for {
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return Value{}, errSyntax
			}
			k, err := p.str()
			if err != nil {
				return Value{}, err
			}
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return Value{}, errSyntax
			}
			p.i++
			p.ws()
			v, err := p.value(depth + 1)
			if err != nil {
				return Value{}, err
			}
			if len(o.Keys) < 16 {
				o.set(k, v)
			} else {
				o.setLarge(k, v)
			}
			p.ws()
			if p.i >= len(p.s) {
				return Value{}, errSyntax
			}
			if p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.s[p.i] == '}' {
				p.i++
				return Value{K: Obj, O: o}, nil
			}
			return Value{}, errSyntax
		}
	case c == '[':
		p.i++
		var a []Value
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return Value{K: Arr, A: []Value{}}, nil
		}
		for {
			p.ws()
			v, err := p.value(depth + 1)
			if err != nil {
				return Value{}, err
			}
			a = append(a, v)
			p.ws()
			if p.i >= len(p.s) {
				return Value{}, errSyntax
			}
			if p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.s[p.i] == ']' {
				p.i++
				return Value{K: Arr, A: a}, nil
			}
			return Value{}, errSyntax
		}
	case c == '"':
		s, err := p.str()
		return Value{K: Str, S: s}, err
	case c == 't':
		return p.lit("true", Value{K: Bool, B: true})
	case c == 'f':
		return p.lit("false", Value{K: Bool})
	case c == 'n':
		return p.lit("null", Value{})
	case c == 'N':
		return p.lit("NaN", Value{K: Float, F: math.NaN()})
	case c == 'I':
		return p.lit("Infinity", Value{K: Float, F: math.Inf(1)})
	case c == '-' && strings.HasPrefix(p.s[p.i:], "-Infinity"):
		return p.lit("-Infinity", Value{K: Float, F: math.Inf(-1)})
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return Value{}, errSyntax
}

// setLarge is set() for big objects (duplicate keys are rare; scan from the end first).
func (o *Object) setLarge(k string, v Value) { o.set(k, v) }

func (p *parser) lit(word string, v Value) (Value, error) {
	if strings.HasPrefix(p.s[p.i:], word) {
		p.i += len(word)
		return v, nil
	}
	return Value{}, errSyntax
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (p *parser) number() (Value, error) {
	st := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	if p.i >= len(p.s) {
		return Value{}, errSyntax
	}
	if p.s[p.i] == '0' {
		p.i++
	} else if p.s[p.i] >= '1' && p.s[p.i] <= '9' {
		for p.i < len(p.s) && isDigit(p.s[p.i]) {
			p.i++
		}
	} else {
		return Value{}, errSyntax
	}
	isFloat := false
	if p.i+1 < len(p.s) && p.s[p.i] == '.' && isDigit(p.s[p.i+1]) {
		isFloat = true
		p.i++
		for p.i < len(p.s) && isDigit(p.s[p.i]) {
			p.i++
		}
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		j := p.i + 1
		if j < len(p.s) && (p.s[j] == '+' || p.s[j] == '-') {
			j++
		}
		if j < len(p.s) && isDigit(p.s[j]) {
			isFloat = true
			p.i = j
			for p.i < len(p.s) && isDigit(p.s[p.i]) {
				p.i++
			}
		}
	}
	lit := p.s[st:p.i]
	if isFloat {
		f, err := strconv.ParseFloat(lit, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return Value{}, errSyntax
		}
		return Value{K: Float, F: f}, nil
	}
	n, err := strconv.ParseInt(lit, 10, 64)
	if err != nil {
		b, ok := new(big.Int).SetString(lit, 10)
		if !ok {
			return Value{}, errSyntax
		}
		return Value{K: Int, Big: b.String()}, nil
	}
	return Value{K: Int, I: n}, nil
}

func hexv(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func (p *parser) hex4(at int) (rune, bool) {
	if at+4 > len(p.s) {
		return 0, false
	}
	var r rune
	for k := 0; k < 4; k++ {
		h := hexv(p.s[at+k])
		if h < 0 {
			return 0, false
		}
		r = r<<4 | rune(h)
	}
	return r, true
}

func (p *parser) str() (string, error) {
	p.i++ // opening quote
	st := p.i
	// fast path: no escapes
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '"' {
			s := p.s[st:p.i]
			p.i++
			return s, nil
		}
		if c == '\\' {
			break
		}
		if c < 0x20 {
			return "", errSyntax
		}
		p.i++
	}
	buf := []byte(p.s[st:p.i])
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return string(buf), nil
		case c < 0x20:
			return "", errSyntax
		case c == '\\':
			if p.i+1 >= len(p.s) {
				return "", errSyntax
			}
			e := p.s[p.i+1]
			p.i += 2
			switch e {
			case '"', '\\', '/':
				buf = append(buf, e)
			case 'b':
				buf = append(buf, '\b')
			case 'f':
				buf = append(buf, '\f')
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case 'u':
				r, ok := p.hex4(p.i)
				if !ok {
					return "", errSyntax
				}
				p.i += 4
				if r >= 0xD800 && r <= 0xDBFF && p.i+6 <= len(p.s) && p.s[p.i] == '\\' && p.s[p.i+1] == 'u' {
					if r2, ok := p.hex4(p.i + 2); ok && r2 >= 0xDC00 && r2 <= 0xDFFF {
						p.i += 6
						r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
					}
				}
				buf = AppendRune(buf, r)
			default:
				return "", errSyntax
			}
		default:
			buf = append(buf, c)
			p.i++
		}
	}
	return "", errSyntax
}

// ---------------------------------------------------------------- json.dumps

// DumpOpts mirrors the json.dumps arguments the reference implementation uses.
type DumpOpts struct {
	EnsureASCII bool // default True in Python
	SortKeys    bool
	Indent      int    // 0 = no indent (single line)
	ItemSep     string // default ", " (or "," with indent)
	KeySep      string // default ": "
}

// Dumps is json.dumps(v) with Python's defaults (ensure_ascii=True, separators (', ', ': ')).
func Dumps(v Value) string { return DumpsOpt(v, DumpOpts{EnsureASCII: true}) }

// DumpsOpt is json.dumps with options.
func DumpsOpt(v Value, o DumpOpts) string {
	if o.ItemSep == "" {
		if o.Indent > 0 {
			o.ItemSep = ","
		} else {
			o.ItemSep = ", "
		}
	}
	if o.KeySep == "" {
		o.KeySep = ": "
	}
	var b []byte
	b = dump(b, v, o, 0)
	return string(b)
}

func dump(b []byte, v Value, o DumpOpts, level int) []byte {
	switch v.K {
	case Null:
		return append(b, "null"...)
	case Bool:
		if v.B {
			return append(b, "true"...)
		}
		return append(b, "false"...)
	case Int:
		if v.Big != "" {
			return append(b, v.Big...)
		}
		return strconv.AppendInt(b, v.I, 10)
	case Float:
		return append(b, FloatRepr(v.F, true)...)
	case Str:
		return QuoteJSON(b, v.S, o.EnsureASCII)
	case Arr:
		if len(v.A) == 0 {
			return append(b, "[]"...)
		}
		b = append(b, '[')
		for i, x := range v.A {
			if i > 0 {
				b = append(b, o.ItemSep...)
			}
			b = newline(b, o, level+1)
			b = dump(b, x, o, level+1)
		}
		b = newline(b, o, level)
		return append(b, ']')
	case Obj:
		if v.O == nil || len(v.O.Keys) == 0 {
			return append(b, "{}"...)
		}
		idx := make([]int, len(v.O.Keys))
		for i := range idx {
			idx[i] = i
		}
		if o.SortKeys {
			sort.SliceStable(idx, func(a, c int) bool { return v.O.Keys[idx[a]] < v.O.Keys[idx[c]] })
		}
		b = append(b, '{')
		for n, i := range idx {
			if n > 0 {
				b = append(b, o.ItemSep...)
			}
			b = newline(b, o, level+1)
			b = QuoteJSON(b, v.O.Keys[i], o.EnsureASCII)
			b = append(b, o.KeySep...)
			b = dump(b, v.O.Vals[i], o, level+1)
		}
		b = newline(b, o, level)
		return append(b, '}')
	}
	return b
}

func newline(b []byte, o DumpOpts, level int) []byte {
	if o.Indent <= 0 {
		return b
	}
	b = append(b, '\n')
	for i := 0; i < level*o.Indent; i++ {
		b = append(b, ' ')
	}
	return b
}

const hexDigits = "0123456789abcdef"

// QuoteJSON appends s as a JSON string literal the way Python's json encoder writes it.
func QuoteJSON(b []byte, s string, ensureASCII bool) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			switch c {
			case '"':
				b = append(b, '\\', '"')
			case '\\':
				b = append(b, '\\', '\\')
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			default:
				if c < 0x20 || (ensureASCII && c == 0x7f) {
					b = append(b, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
				} else {
					b = append(b, c)
				}
			}
			i++
			continue
		}
		r, sz := DecodeRune(s[i:])
		if ensureASCII || (r >= 0xD800 && r <= 0xDFFF) {
			b = appendU(b, r)
		} else {
			b = append(b, s[i:i+sz]...)
		}
		i += sz
	}
	return append(b, '"')
}

func appendU(b []byte, r rune) []byte {
	if r >= 0x10000 {
		r -= 0x10000
		b = appendU4(b, 0xD800+(r>>10))
		return appendU4(b, 0xDC00+(r&0x3FF))
	}
	return appendU4(b, r)
}

func appendU4(b []byte, r rune) []byte {
	return append(b, '\\', 'u', hexDigits[r>>12&0xF], hexDigits[r>>8&0xF], hexDigits[r>>4&0xF], hexDigits[r&0xF])
}

// FloatRepr is Python's repr(float). json=true gives json.dumps spelling of non-finite values
// (NaN, Infinity, -Infinity); otherwise Python's repr (nan, inf, -inf).
func FloatRepr(f float64, json bool) string {
	switch {
	case math.IsNaN(f):
		if json {
			return "NaN"
		}
		return "nan"
	case math.IsInf(f, 1):
		if json {
			return "Infinity"
		}
		return "inf"
	case math.IsInf(f, -1):
		if json {
			return "-Infinity"
		}
		return "-inf"
	}
	// shortest round-trip digits, then Python's layout: fixed for 1e-4 <= |x| < 1e16, else exponent
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddde±XX
	mant, exps, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(exps)
	neg := strings.HasPrefix(mant, "-")
	if neg {
		mant = mant[1:]
	}
	digits := strings.Replace(mant, ".", "", 1)
	var out string
	if exp >= -4 && exp < 16 {
		out = layoutFixed(digits, exp)
	} else {
		out = digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		sign := "+"
		if exp < 0 {
			sign = "-"
			exp = -exp
		}
		es := strconv.Itoa(exp)
		if len(es) < 2 {
			es = "0" + es
		}
		out += "e" + sign + es
	}
	if neg {
		out = "-" + out
	}
	return out
}

func layoutFixed(digits string, exp int) string {
	// value = 0.digits * 10^(exp+1)
	point := exp + 1
	switch {
	case point <= 0:
		return "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		return digits + strings.Repeat("0", point-len(digits)) + ".0"
	default:
		return digits[:point] + "." + digits[point:]
	}
}

// ---------------------------------------------------------------- str() / repr()

// PyStr is Python's str(v) for a decoded JSON value.
func PyStr(v Value) string {
	if v.K == Str {
		return v.S
	}
	return Repr(v)
}

// Repr is Python's repr(v) for a decoded JSON value (None, True, 1.5, 'text', [..], {..}).
func Repr(v Value) string {
	var b []byte
	return string(repr(b, v))
}

func repr(b []byte, v Value) []byte {
	switch v.K {
	case Null:
		return append(b, "None"...)
	case Bool:
		if v.B {
			return append(b, "True"...)
		}
		return append(b, "False"...)
	case Int:
		if v.Big != "" {
			return append(b, v.Big...)
		}
		return strconv.AppendInt(b, v.I, 10)
	case Float:
		return append(b, FloatRepr(v.F, false)...)
	case Str:
		return ReprStr(b, v.S)
	case Arr:
		b = append(b, '[')
		for i, x := range v.A {
			if i > 0 {
				b = append(b, ", "...)
			}
			b = repr(b, x)
		}
		return append(b, ']')
	case Obj:
		b = append(b, '{')
		if v.O != nil {
			for i, k := range v.O.Keys {
				if i > 0 {
					b = append(b, ", "...)
				}
				b = ReprStr(b, k)
				b = append(b, ": "...)
				b = repr(b, v.O.Vals[i])
			}
		}
		return append(b, '}')
	}
	return b
}

// ReprStr appends Python's repr(str).
func ReprStr(b []byte, s string) []byte {
	q := byte('\'')
	if strings.IndexByte(s, '\'') >= 0 && strings.IndexByte(s, '"') < 0 {
		q = '"'
	}
	b = append(b, q)
	for i := 0; i < len(s); {
		r, sz := DecodeRune(s[i:])
		switch {
		case r == rune(q) || r == '\\':
			b = append(b, '\\', byte(r))
		case r == '\n':
			b = append(b, '\\', 'n')
		case r == '\r':
			b = append(b, '\\', 'r')
		case r == '\t':
			b = append(b, '\\', 't')
		case r < 0x20 || r == 0x7f:
			b = append(b, '\\', 'x', hexDigits[r>>4], hexDigits[r&0xF])
		case r < 0x7f:
			b = append(b, byte(r))
		case !isPrintable(r):
			switch {
			case r < 0x100:
				b = append(b, '\\', 'x', hexDigits[r>>4], hexDigits[r&0xF])
			case r < 0x10000:
				b = append(b, '\\', 'u', hexDigits[r>>12&0xF], hexDigits[r>>8&0xF], hexDigits[r>>4&0xF], hexDigits[r&0xF])
			default:
				b = append(b, '\\', 'U', '0', '0', hexDigits[r>>20&0xF], hexDigits[r>>16&0xF], hexDigits[r>>12&0xF], hexDigits[r>>8&0xF], hexDigits[r>>4&0xF], hexDigits[r&0xF])
			}
		default:
			b = append(b, s[i:i+sz]...)
		}
		i += sz
	}
	return append(b, q)
}

// isPrintable approximates str.isprintable() for one non-ASCII code point.
func isPrintable(r rune) bool {
	if r >= 0xD800 && r <= 0xDFFF {
		return false
	}
	return unicode.IsPrint(r)
}
