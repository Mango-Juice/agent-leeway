package leak

import (
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// F fills a message template.
func F(tmpl string, kv ...any) string {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return py.Format(tmpl, m)
}

func toolName(t string) string {
	if n, ok := messages.Tool[t]; ok {
		return n
	}
	return t
}

func pct(X *Report) float64 {
	if X.TotalUSD != 0 {
		return X.LeakUSD / X.TotalUSD
	}
	return 0
}

func date(ns int64, loc *time.Location) string { return time.Unix(0, ns).In(loc).Format("2006-01-02") }

// when is a session's in-window activity range (ISO dates, local time), noting an earlier start.
func when(s SessionLeak, loc *time.Location) string {
	f, l := date(s.First, loc), date(s.Last, loc)
	rng := f
	if f != l {
		rng = f + ".." + l
	}
	if s.StartedBefore && s.Start != logs.NaT {
		return F(messages.Leak["started_before"], "rng", rng, "start", date(s.Start, loc))
	}
	return rng
}

// Render is the terminal text of the leak check. P, H, A, PH, PA may be nil (no previous window / no split).
func Render(R *Report, top int, P, H, A, PH, PA *Report, now time.Time, loc *time.Location) string {
	T := messages.Leak
	since := R.WindowStart.In(loc)
	until := R.WindowEnd
	if now.Before(until) {
		until = now
	}
	until = until.In(loc)
	title := F(T["title"], "since", since.Format("2006-01-02 15:04"), "until", until.Format("2006-01-02 15:04 -0700"))
	if R.Tool != "all" {
		title += F(T["only_tool"], "tool", messages.Tool[R.Tool])
	}
	out := []string{title, messages.Money(config.Prices().Read), messages.WindowNote}
	if note := config.StalePricesNote(now); note != "" && !config.Legacy {
		out = append(out, note)
	}
	var bt []string
	for _, x := range R.ByTool {
		bt = append(bt, toolName(x[0].(string))+" $"+py.FormatValue(x[1], ",.2f"))
	}
	if len(bt) == 0 {
		out = append(out, T["spend_none"])
	} else {
		out = append(out, F(T["spend"], "total", R.TotalUSD, "by_tool", strings.Join(bt, " · "), "leak", R.LeakUSD, "pct", pct(R)))
	}
	if P != nil && P.TotalUSD != 0 {
		out = append(out, F(T["prev"], "total", P.TotalUSD, "leak", P.LeakUSD, "pct", pct(P), "a", 100*pct(P), "b", 100*pct(R)))
	}
	if H != nil && A != nil {
		for _, x := range []struct {
			name string
			X    *Report
		}{{T["origin_human"], H}, {T["origin_auto"], A}} {
			out = append(out, F(T["origin_line"], "name", x.name, "total", x.X.TotalUSD, "leak", x.X.LeakUSD, "pct", pct(x.X)))
		}
		out = append(out, T["origin_note"])
	}
	HB, HP := R, P
	titleKey := "habits_title_all"
	if H != nil {
		HB, HP, titleKey = H, PH, "habits_title"
	}
	out = append(out, "", T[titleKey])
	prev := map[string]Habit{}
	if HP != nil {
		for _, h := range HP.Habits {
			prev[h.Habit] = h
		}
	}
	hs := append([]Habit(nil), HB.Habits...)
	sort.SliceStable(hs, func(a, b int) bool { return hs[a].USD > hs[b].USD })
	for _, h := range hs {
		if h.Count == 0 {
			continue
		}
		pv := ""
		if p, ok := prev[h.Habit]; ok && HP.TotalUSD != 0 {
			pv = F(T["habit_prev"], "v", p.Per100)
		}
		out = append(out, F(T["habit_line"], "label", h.Label, "n", h.Count, "usd", h.USD, "per100", h.Per100, "prev", pv))
		if len(h.Where) > 0 {
			w := h.Where
			out = append(out, F(T["habit_where"], "wd", w["weekday"].Top, "wds", w["weekday"].Share, "tb", w["time"].Top, "tbs", w["time"].Share,
				"pj", w["project"].Top, "pjs", w["project"].Share))
		}
		out = append(out, F(T["habit_change"], "tip", h.Tip))
	}
	if A != nil && len(A.AutomationFixes) > 0 {
		out = append(out, "", T["auto_title"])
		for _, f := range A.AutomationFixes {
			out = append(out, F(T["auto_line"], "label", f.Label, "cost", f.Cost, "leak", f.Leak))
			for _, tip := range f.Tips {
				out = append(out, "      - "+tip)
			}
		}
	}
	split := ""
	if H != nil {
		split = T["causes_split"]
	}
	out = append(out, "", T["causes_title"]+split)
	hc, ac := map[string]Cause{}, map[string]Cause{}
	if H != nil {
		for _, c := range H.Causes {
			hc[c.Cause] = c
		}
	}
	if A != nil {
		for _, c := range A.Causes {
			ac[c.Cause] = c
		}
	}
	for _, c := range R.Causes {
		extra := ""
		if len(hc) > 0 {
			extra = F(T["cause_split"], "hu", hc[c.Cause].USD, "hn", hc[c.Cause].Count, "au", ac[c.Cause].USD, "an", ac[c.Cause].Count)
		}
		out = append(out, F(T["cause_line"], "usd", c.USD, "n", c.Count, "label", c.Label, "extra", extra))
	}
	for _, c := range R.Causes {
		if c.Cause == "reset_unknown" && c.Count != 0 {
			out = append(out, F(T["unknown_note"], "usd", c.USD))
		}
	}
	out = append(out, "", T["info_title"])
	if R.TotalUSD != 0 {
		out = append(out, F(T["info_subagent"], "usd", R.Info.Subagent, "share", R.Info.Subagent/R.TotalUSD))
	}
	out = append(out, F(T["info_partial"], "usd", R.Info.PartialMiss, "n", R.Info.PartialCount))
	if R.Info.Uncertain > 0 {
		out = append(out, F(T["info_uncertain"], "n", R.Info.Uncertain, "usd", R.Info.UncertainUSD))
	}
	for _, a := range R.Info.Assumed {
		fb, _ := config.Prices().Lookup(a.Tool, "")
		out = append(out, F(T["info_assumed"], "n", a.Responses, "share", a.TokenShare, "model", a.Model, "fallback", fb.ID))
	}
	out = append(out, "", T["sessions_title"])
	n := 0
	for _, s := range R.Sessions {
		if s.Leak < 0.01 {
			continue
		}
		if n >= top {
			break
		}
		n++
		type part struct {
			name string
			v    float64
		}
		var parts []part
		for _, k := range causeKeys() {
			if s.ByCause[k] >= 0.01 {
				parts = append(parts, part{messages.ShortCause[k], s.ByCause[k]})
			}
		}
		sort.SliceStable(parts, func(a, b int) bool { return parts[a].v > parts[b].v })
		if len(parts) > 3 {
			parts = parts[:3]
		}
		var ps []string
		for _, p := range parts {
			ps = append(ps, p.name+" $"+py.FormatValue(p.v, ",.2f"))
		}
		tag := ""
		if s.AutoShare != 0 {
			if s.AutoShare > 0.5 {
				tag = T["tag_auto"]
			} else {
				tag = F(T["tag_mixed"], "share", s.AutoShare)
			}
		}
		out = append(out, F(T["session_line"], "when", when(s, loc), "tag", tag, "tool", toolName(s.Tool), "project", s.Project,
			"leak", s.Leak, "cost", s.Cost, "parts", strings.Join(ps, ", ")))
	}
	out = append(out, "", T["keys_note"])
	return strings.Join(out, "\n")
}
