package py

import (
	"os"
	"testing"
)

func load(t *testing.T) Value {
	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := Loads(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func goVal(v Value) any {
	switch v.K {
	case Int:
		return v.I
	case Float:
		return v.F
	case Str:
		return v.S
	}
	return v
}

func TestFormatVectors(t *testing.T) {
	V := load(t)
	for _, c := range V.Get("fmt").A {
		got := FormatValue(goVal(c.A[0]), c.A[1].S)
		if got != c.A[2].S {
			t.Errorf("format(%s, %q) = %q, Python %q", Repr(c.A[0]), c.A[1].S, got, c.A[2].S)
		}
	}
	for _, c := range V.Get("str").A {
		if got := FormatValue(c.A[0].S, c.A[1].S); got != c.A[2].S {
			t.Errorf("format(%q, %q) = %q, Python %q", c.A[0].S, c.A[1].S, got, c.A[2].S)
		}
	}
	for _, c := range V.Get("round").A {
		n, _ := c.A[1].Num()
		want, _ := c.A[2].Num()
		f, _ := c.A[0].Num()
		if got := Round(f, int(n)); got != want {
			t.Errorf("round(%v, %v) = %v, Python %v", f, n, got, want)
		}
	}
}

func TestDumpsReprVectors(t *testing.T) {
	V := load(t)
	for _, c := range V.Get("dumps").A {
		v := c.A[0]
		checks := []struct{ name, got, want string }{
			{"dumps", Dumps(v), c.A[1].S},
			{"dumps sort_keys", DumpsOpt(v, DumpOpts{EnsureASCII: true, SortKeys: true}), c.A[2].S},
			{"dumps ensure_ascii=False", DumpsOpt(v, DumpOpts{}), c.A[3].S},
			{"str", PyStr(v), c.A[4].S},
			{"repr", Repr(v), c.A[5].S},
		}
		for _, k := range checks {
			if k.got != k.want {
				t.Errorf("%s: got %q, Python %q", k.name, k.got, k.want)
			}
		}
	}
	for _, c := range V.Get("lens").A {
		s, err := Loads(c.A[0].S)
		if err != nil {
			t.Fatal(err)
		}
		if got := Len(s.S); int64(got) != c.A[1].I {
			t.Errorf("len(%s) = %d, Python %d", c.A[0].S, got, c.A[1].I)
		}
	}
}

func TestLoadsPython(t *testing.T) {
	bad := []string{"", "{", "[1,]", `{"a":1,}`, "\xef\xbb\xbf{}", `"a` + "\x01" + `"`, "01", "1.", "tru", `{"a" 1}`}
	for _, s := range bad {
		if _, err := Loads(s); err == nil {
			t.Errorf("Loads(%q) should fail like json.loads", s)
		}
	}
	good := map[string]string{" [1, 2.5, -0, 1e2] \n": "[1, 2.5, 0, 100.0]", `{"a":1,"a":2,"b":3}`: "{'a': 2, 'b': 3}",
		`NaN`: "nan", `"😀"`: "'😀'", `[true,false,null]`: "[True, False, None]"}
	for in, want := range good {
		v, err := Loads(in)
		if err != nil {
			t.Fatalf("Loads(%q): %v", in, err)
		}
		if got := Repr(v); got != want {
			t.Errorf("repr(json.loads(%q)) = %s, Python %s", in, got, want)
		}
	}
	// a lone surrogate survives a round trip and counts as one code point
	v, _ := Loads(`"a\udc80b"`)
	if Len(v.S) != 3 || Dumps(v) != `"a\udc80b"` {
		t.Errorf("lone surrogate: len %d dumps %s", Len(v.S), Dumps(v))
	}
}

func TestDecodeBytesLikePython(t *testing.T) {
	// b'\xe3\x81 x \xff \xed\xa0\x80'.decode('utf-8', 'replace') == '� x � ���'
	got := DecodeBytes([]byte("\xe3\x81 x \xff \xed\xa0\x80"))
	want := "� x � ���"
	if got != want {
		t.Errorf("DecodeBytes = %q, want %q", got, want)
	}
}

func TestTextHelpers(t *testing.T) {
	if Strip("　 a b\x1c\n") != "a b" || Len("가나") != 2 || Head("가나다", 2) != "가나" || Tail("가나다", 1) != "다" {
		t.Error("strip/len/head/tail")
	}
	if got := Fields(" a b\tc "); len(got) != 3 {
		t.Errorf("Fields = %q", got)
	}
	if v, nz, _ := ParseInt("١٢"); v != 12 || !nz { // Arabic-Indic digits, as int() reads them
		t.Errorf("ParseInt = %d", v)
	}
}
