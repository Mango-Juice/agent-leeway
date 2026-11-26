package py

import (
	"reflect"
	"testing"
	"unsafe"
)

// LoadsOwned gives the same value as Loads, and its strings are copies: none points into the parsed text.
func TestLoadsOwned(t *testing.T) {
	s := `{"a":"plain","b":"esc\"aped","c":[1,2.5,"x",{"d":null,"e":true}],"big":123456789012345678901234567890,"ü":"ü"}`
	a, errA := Loads(s)
	b, errB := LoadsOwned(s)
	if errA != nil || errB != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("differ: %v %v", errA, errB)
	}
	lo := uintptr(unsafe.Pointer(unsafe.StringData(s)))
	hi := lo + uintptr(len(s))
	var walk func(v Value)
	walk = func(v Value) {
		check := func(x string) {
			if len(x) > 0 {
				if p := uintptr(unsafe.Pointer(unsafe.StringData(x))); p >= lo && p < hi {
					t.Errorf("%q shares memory with the input", x)
				}
			}
		}
		check(v.S)
		for _, x := range v.A {
			walk(x)
		}
		if v.O != nil {
			for i, k := range v.O.Keys {
				check(k)
				walk(v.O.Vals[i])
			}
		}
	}
	walk(b)
	for _, bad := range []string{`{"a":`, `[1,]`, `"x`, ``} {
		_, e1 := Loads(bad)
		_, e2 := LoadsOwned(bad)
		if (e1 == nil) != (e2 == nil) {
			t.Errorf("%q: %v / %v", bad, e1, e2)
		}
	}
}
