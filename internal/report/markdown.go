package report

import (
	"sort"
	"strings"
)

// mq neutralizes text for Markdown: one line, no raw HTML, no links or images (a remote image would make a
// viewer contact a third party), no code spans, no table breaks.
func mq(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.NewReplacer("<", "&lt;", ">", "&gt;", "![", "!\\[", "](", "]\\(", "`", "'", "|", "\\|").Replace(s)
}

func code(s string) string { return "`" + strings.ReplaceAll(s, "`", "'") + "`" }

func link(title, url string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return "[" + mq(title) + "](" + url + ")"
	}
	return mq(title) + " (" + mq(url) + ")"
}

// sessionsWorstFirst orders sessions with events by verdict, then by number of events.
func sessionsWorstFirst(m *Model) []*Record {
	order := map[string]int{}
	for i, v := range Verdicts {
		order[v] = i
	}
	var rs []*Record
	for _, r := range m.Records {
		if len(r.Events) > 0 {
			rs = append(rs, r)
		}
	}
	sort.SliceStable(rs, func(a, b int) bool {
		oa, ok := order[rs[a].Verdict]
		if !ok {
			oa = 9
		}
		ob, ok := order[rs[b].Verdict]
		if !ok {
			ob = 9
		}
		if oa != ob {
			return oa < ob
		}
		return len(rs[a].Events) > len(rs[b].Events)
	})
	return rs
}

func leaksByUSD(m *Model) []*Finding {
	ls := append([]*Finding(nil), m.Leak...)
	sort.SliceStable(ls, func(a, b int) bool { return ls[a].USD > ls[b].USD })
	return ls
}

func countVerdict(m *Model, v string) int {
	n := 0
	for _, j := range m.Verdicts {
		n += j.Counts[v]
	}
	return n
}

func adviceMD(x Text, f *Finding) []string {
	var md []string
	for _, a := range f.Advice {
		tool := ""
		if len(f.Advice) > 1 {
			tool = " (" + x.Tool(a.Tool) + ")"
		}
		md = append(md, "- **"+x.T["do"]+"**"+tool+": "+x.Do(a, code))
		for _, s := range a.A.Sources {
			md = append(md, "  - "+x.Basis(a.A.Basis)+": "+link(s.Title, s.URL)+", "+s.Read+": “"+mq(s.Claim)+"”")
		}
	}
	for _, a := range f.Against {
		md = append(md, "- "+x.T["against"]+": "+pickLabel(a.Note, x.Lang)+" ("+link(a.Title, a.URL)+")")
	}
	return md
}

func whereText(f *Finding) string {
	var parts []string
	for _, k := range []string{"weekday", "time", "project"} {
		if w, ok := f.Where[k]; ok && w.Top != "" {
			parts = append(parts, w.Top+" "+Pct(w.Share))
		}
	}
	return strings.Join(parts, ", ")
}

func joinPre(xs []string, sep string) string {
	if len(xs) == 0 {
		return ""
	}
	return sep + strings.Join(xs, sep)
}

func pickLabel(m map[string]string, lang string) string {
	if s := m[lang]; s != "" {
		return s
	}
	return m["en"]
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
