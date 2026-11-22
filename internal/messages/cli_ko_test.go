package messages

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every user-facing line of drift and report follows --lang: every Drift and Cli key has Korean text with the
// same placeholders, except the keys that are judge input or stored data (KeepEnglish).
func TestKoreanCoversDriftAndCli(t *testing.T) {
	ph := regexp.MustCompile(`\{[a-z_]+(:[^}]*)?\}`)
	set := func(s string) string {
		m := ph.FindAllString(s, -1)
		sort.Strings(m)
		return strings.Join(m, ",")
	}
	for name, tables := range map[string][2]map[string]string{"Drift": {driftEN, driftKO}, "Cli": {cliEN, cliKO}} {
		en, ko := tables[0], tables[1]
		for k, v := range en {
			if KeepEnglish[k] {
				continue
			}
			w, ok := ko[k]
			if !ok {
				t.Errorf("%s[%q] has no Korean text", name, k)
				continue
			}
			if set(v) != set(w) {
				t.Errorf("%s[%q]: placeholders differ: en %s, ko %s", name, k, set(v), set(w))
			}
		}
		for k := range ko {
			if _, ok := en[k]; !ok {
				t.Errorf("%s[%q] is Korean only", name, k)
			}
		}
	}
	SetLang("ko")
	if Drift["truncated"] != driftEN["truncated"] || Drift["head"] == driftEN["head"] || !strings.Contains(Privacy, "개인정보") {
		t.Error("SetLang(ko) must switch the interface text and keep the judge-input text")
	}
	SetLang("en")
	if Drift["head"] != driftEN["head"] || Privacy != privacyEN {
		t.Error("SetLang(en) must restore English")
	}
}
