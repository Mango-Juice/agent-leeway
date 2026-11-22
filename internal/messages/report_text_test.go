package messages

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestReportTextParity(t *testing.T) {
	ph := regexp.MustCompile(`\{[a-z_]+\}`)
	set := func(s string) string {
		m := ph.FindAllString(s, -1)
		sort.Strings(m)
		return strings.Join(m, ",")
	}
	en, ko := ReportText["en"], ReportText["ko"]
	for k, v := range en {
		w, ok := ko[k]
		if !ok {
			t.Errorf("ko lacks %q", k)
			continue
		}
		if set(v) != set(w) {
			t.Errorf("%q: placeholders differ: en %s, ko %s", k, set(v), set(w))
		}
	}
	for k := range ko {
		if _, ok := en[k]; !ok {
			t.Errorf("en lacks %q", k)
		}
	}
}

func TestStoryTextParity(t *testing.T) {
	ph := regexp.MustCompile(`\{[a-z_]+\}`)
	set := func(s string) string {
		m := ph.FindAllString(s, -1)
		sort.Strings(m)
		u := m[:0]
		for i, x := range m {
			if i == 0 || x != m[i-1] {
				u = append(u, x)
			}
		}
		return strings.Join(u, ",")
	}
	en, ko := Story["en"], Story["ko"]
	if len(en) != len(ko) {
		t.Fatalf("%d en keys, %d ko keys", len(en), len(ko))
	}
	for k, v := range en {
		if set(v) != set(ko[k]) {
			t.Errorf("%q: placeholders differ: en %s, ko %s", k, set(v), set(ko[k]))
		}
	}
}
