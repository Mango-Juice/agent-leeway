package pipeline

import (
	"math"
	"sort"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// WasteRow is the mechanical waste of one response (step_waste).
type WasteRow struct {
	Key                                string
	Tool, ThreadID                     string
	Seq                                int
	TS                                 int64
	WCost, Fixed, MissCost, MissExpiry float64
	Sub, AfterCompaction               bool
	CallWaste, CFail, CRepeat, CReread float64
	NFail, NRepeat, NReread, NOversize int
	NExpiry, NMidmiss                  int
	Oversize                           float64
	// Assumed is the model name when the response was priced with a fallback row; Tokens are its input and output.
	Assumed string
	Tokens  int64
	// kept for the reset table (same values the reference recomputes there)
	prevIn float64
	prevTS int64
	step   *StepRow
}

type tsKey struct {
	thread string
	seq    int
}

// BuildWaste computes per-response waste: cache misses (expiry or mid-session), oversized tool output,
// repeated commands, consecutive failures and unchanged re-reads (waste.build).
func (t *Tables) BuildWaste() {
	t.Waste = nil
	keep := map[string]bool{}
	for _, s := range t.Sessions {
		if !s.Exclude {
			keep[s.Key] = true
		}
	}
	if len(t.Sessions) == 0 || len(t.Steps) == 0 {
		return
	}
	userThreads := map[string]bool{}
	for _, x := range t.Threads {
		if x.Source.OK && x.Source.S == "user" {
			userThreads[x.ThreadID] = true
		}
	}
	var st []*StepRow
	for i := range t.Steps {
		if s := &t.Steps[i]; s.Key.OK && keep[s.Key.S] {
			st = append(st, s)
		}
	}
	sort.SliceStable(st, func(a, b int) bool {
		if st[a].ThreadID != st[b].ThreadID {
			return st[a].ThreadID < st[b].ThreadID
		}
		return st[a].Seq < st[b].Seq
	})
	var ca []*CallRow
	for i := range t.Calls {
		if c := &t.Calls[i]; c.Key.OK && keep[c.Key.S] {
			ca = append(ca, c)
		}
	}
	sort.SliceStable(ca, func(a, b int) bool {
		x, y := ca[a], ca[b]
		if x.ThreadID != y.ThreadID {
			return x.ThreadID < y.ThreadID
		}
		if x.TS != y.TS {
			return lessTS(x.TS, y.TS)
		}
		return x.Step < y.Step
	})
	// per-response cost index and the number of the thread's last response
	stepByKey := map[tsKey]*StepRow{}
	maxSeq := map[string]int{}
	for _, s := range st {
		stepByKey[tsKey{s.ThreadID, s.Seq}] = s
		if m, ok := maxSeq[s.ThreadID]; !ok || s.Seq > m {
			maxSeq[s.ThreadID] = s.Seq
		}
	}
	rows := make([]WasteRow, len(st))
	index := map[tsKey]int{}
	for i, s := range st {
		r := WasteRow{Key: s.Key.S, Tool: s.Tool, ThreadID: s.ThreadID, Seq: s.Seq, TS: s.TS, WCost: s.WCost,
			Sub: !userThreads[s.ThreadID], AfterCompaction: s.AfterCompaction, prevIn: NaN, prevTS: NaT, step: s,
			Assumed: s.Assumed, Tokens: s.InputTotal + s.Output}
		inCost := (float64(s.Uncached)*s.PIn + float64(s.Cached)*s.PRead + float64(s.CacheWrite)*s.PWrite) / 1e6
		if s.Seq == 0 {
			r.Fixed = inCost
		}
		if i > 0 && st[i-1].ThreadID == s.ThreadID {
			p := st[i-1]
			r.prevIn, r.prevTS = float64(p.InputTotal), p.TS
		}
		// (a) cache miss: prefix already sent in the previous response but not read from cache
		miss := 0.0
		if !math.IsNaN(r.prevIn) && !s.AfterCompaction {
			miss = math.Max(math.Min(r.prevIn, float64(s.InputTotal))-float64(s.Cached), 0)
		}
		paidTok := s.Uncached + s.CacheWrite
		paidPrice := s.PIn
		if paidTok > 0 {
			paidPrice = (float64(s.Uncached)*s.PIn + float64(s.CacheWrite)*s.PWrite) / float64(paidTok)
		}
		r.MissCost = miss * math.Max(paidPrice-s.PRead, 0) / 1e6
		ttl, ok := s.TTL, s.TTL > 0
		if !ok {
			ttl, ok = config.LegacyCacheTTL[s.Tool]
		}
		if ok && s.TS != NaT && r.prevTS != NaT && s.TS-r.prevTS > int64(ttl) {
			r.MissExpiry = r.MissCost
		}
		rows[i] = r
		index[tsKey{s.ThreadID, s.Seq}] = i
	}
	// per-call flags, thread by thread in (ts, step) order
	type flags struct{ fail, repeat, reread bool }
	fl := make([]flags, len(ca))
	execLike := map[string]bool{"exec": true, "read": true, "search": true, "commit": true, "mcp": true}
	for i := 0; i < len(ca); {
		j := i
		for j < len(ca) && ca[j].ThreadID == ca[i].ThreadID {
			j++
		}
		prevFailed := false
		seen := map[string]int{}
		lastRead := map[[2]string]int{}
		editCount := map[string]int{}
		nsKey := func(n logs.NS) string {
			if !n.OK {
				return "\x00none"
			}
			return "s" + n.S
		}
		for k := i; k < j; k++ {
			c := ca[k]
			if c.Kind == "edit" || c.Kind == "edit_record" {
				editCount[nsKey(c.Path)]++
				continue
			}
			if !execLike[c.Kind] {
				continue
			}
			failed := c.Failed == logs.TriTrue
			if failed && prevFailed {
				fl[k].fail = true // 2nd+ consecutive failure
			}
			prevFailed = failed
			tk := nsKey(c.Target)
			seen[tk]++
			if seen[tk] >= 3 {
				fl[k].repeat = true // same command 3rd+ time
			}
			if c.Kind == "read" && c.Target.Truthy() {
				rk := [2]string{tk, nsKey(c.Path)}
				if n, ok := lastRead[rk]; ok && n == editCount[nsKey(c.Path)] {
					fl[k].reread = true // same read, file unchanged
				}
				lastRead[rk] = editCount[nsKey(c.Path)]
			}
		}
		i = j
	}
	// step cost shared by the calls of that step (wrappers excluded: their real commands carry the cost)
	nIn := map[tsKey]int{}
	for _, c := range ca {
		if c.Kind != "wrapper" {
			nIn[tsKey{c.ThreadID, c.Step}]++
		}
	}
	type sums struct {
		waste, fail, repeat, reread kahan
		nf, nr, nrr                 int
		nOver                       int
		over                        kahan
	}
	S := map[tsKey]*sums{}
	get := func(k tsKey) *sums {
		x := S[k]
		if x == nil {
			x = &sums{}
			S[k] = x
		}
		return x
	}
	for k, c := range ca {
		tk := tsKey{c.ThreadID, c.Step}
		if c.Kind != "wrapper" {
			stepCost := 0.0
			if s, ok := stepByKey[tk]; ok {
				stepCost = s.WCost
			}
			callCost := stepCost / float64(nIn[tk])
			f := fl[k]
			x := get(tk)
			if f.fail || f.repeat || f.reread {
				x.waste.add(callCost)
			} else {
				x.waste.add(0)
			}
			switch {
			case f.fail:
				x.fail.add(callCost)
				x.nf++
			case f.repeat:
				x.repeat.add(callCost)
				x.nr++
			case f.reread:
				x.reread.add(callCost)
				x.nrr++
			}
		}
		// (b) oversized tool output, only outputs the model received: Codex item_completed records
		// ('command', 'file_change', 'mcp:*') hold raw process output that Codex truncates first
		name := ""
		if c.Name.OK {
			name = c.Name.S
		}
		itemDerived := c.Tool == "codex" && (name == "command" || name == "file_change" || strings.HasPrefix(name, "mcp:"))
		if itemDerived {
			continue
		}
		pIn, pRead, remaining := 1.25, 0.125, 0.0 // the reference's defaults when the call has no response
		if !config.Legacy {
			fb, _ := config.Prices().Lookup(c.Tool, "")
			pIn, pRead = fb.Input, fb.CacheRead
		}
		if s, ok := stepByKey[tk]; ok {
			pIn, pRead = s.PIn, s.PRead
			remaining = float64(maxSeq[s.ThreadID] - s.Seq)
		}
		excess := math.Max(float64(c.OutChars-config.ExcessOutputChars), 0) / config.CharsPerToken
		cost := excess * (pIn + pRead*math.Min(remaining, 200)) / 1e6
		x := get(tk)
		x.over.add(cost)
		if c.OutChars > config.ExcessOutputChars {
			x.nOver++
		}
	}
	for i := range rows {
		r := &rows[i]
		if x, ok := S[tsKey{r.ThreadID, r.Seq}]; ok {
			r.CallWaste, r.CFail, r.CRepeat, r.CReread = x.waste.sum, x.fail.sum, x.repeat.sum, x.reread.sum
			r.NFail, r.NRepeat, r.NReread, r.NOversize = x.nf, x.nr, x.nrr, x.nOver
			r.Oversize = x.over.sum
		}
		if r.MissExpiry > 0 {
			r.NExpiry = 1
		}
		if r.MissCost-r.MissExpiry > 1e-9 {
			r.NMidmiss = 1
		}
	}
	t.Waste = rows
}
