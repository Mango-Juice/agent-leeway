package cli

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Made-up Claude Code logs for "Unused skills": 22 direct sessions and one automation that uses release-notes in a month with the same skill list (plus one skill
// added in the last three). svg-icons is called by the agent, pdf-forms typed as a command, ledger:export typed
// with its plugin prefix; the atlas plugin's three skills are never used. With over, the list was full and some
// descriptions were dropped.
func idleLogs(t *testing.T, root string, over bool) {
	t.Helper()
	described := func(name string) bool {
		return !over || (name != "gif-maker" && name != "atlas:tiles" && name != "release-notes")
	}
	base := []string{"svg-icons", "pdf-forms", "gif-maker", "release-notes", "atlas:maps", "atlas:tiles", "atlas:routes", "ledger:export", "ledger:audit"}
	t0 := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 23; i++ { // the last one is an automation (sdk-cli) that uses release-notes
		sid := fmt.Sprintf("sess-idle-%04d", i)
		at := t0.Add(time.Duration(min(i, 21)) * 31 * time.Hour).Add(time.Duration(i/22) * time.Hour)
		ts := func(k int) string {
			return at.Add(time.Duration(k) * 30 * time.Second).Format("2006-01-02T15:04:05.000Z")
		}
		cwd := "/home/dev/trail-planner" // privacy-ok: made-up path
		line := func(typ string, k int, extra map[string]any) map[string]any {
			entry := "cli"
			if i == 22 {
				entry = "sdk-cli"
			}
			m := map[string]any{"type": typ, "sessionId": sid, "cwd": cwd, "version": "2.1.300", "entrypoint": entry, "timestamp": ts(k), "uuid": fmt.Sprintf("%s-%d", sid, k)}
			for a, b := range extra {
				m[a] = b
			}
			return m
		}
		names := append([]string{}, base...)
		if i >= 19 && i < 22 {
			names = append(names, "late-skill")
		}
		var content []string
		for _, n := range names {
			if described(n) {
				content = append(content, "- "+n+": "+strings.Repeat("Does one thing well. ", 3))
			} else {
				content = append(content, "- "+n)
			}
		}
		user := func(k int, text string) map[string]any {
			return line("user", k, map[string]any{"origin": map[string]any{"kind": "human"}, "promptSource": "typed", "message": map[string]any{"role": "user", "content": text}})
		}
		step := func(k int, c []any) map[string]any {
			return line("assistant", k, map[string]any{"message": map[string]any{"id": fmt.Sprintf("%s-m%d", sid, k), "model": "claude-opus-5-5", "role": "assistant", "content": c,
				"usage": map[string]any{"input_tokens": 10, "cache_read_input_tokens": 20000, "cache_creation_input_tokens": 2000, "output_tokens": 300}}})
		}
		recs := []map[string]any{
			line("attachment", 0, map[string]any{"attachment": map[string]any{"type": "skill_listing", "isInitial": true, "skillCount": len(names), "names": names, "content": strings.Join(content, "\n")}}),
			user(1, "tidy up the map page"),
			step(2, []any{map[string]any{"type": "text", "text": "done"}}),
		}
		switch i {
		case 3:
			recs = append(recs, step(3, []any{map[string]any{"type": "tool_use", "id": sid + "-t", "name": "Skill", "input": map[string]any{"skill": "svg-icons"}}}))
		case 8:
			recs = append(recs, user(3, "<command-message>pdf-forms</command-message>\n<command-name>/pdf-forms</command-name>\n<command-args>the March form</command-args>"))
		case 22:
			recs = append(recs, step(3, []any{map[string]any{"type": "tool_use", "id": sid + "-t", "name": "Skill", "input": map[string]any{"skill": "release-notes"}}}))
		case 12:
			recs = append(recs, user(3, "<command-name>/ledger:export</command-name>"), user(4, "<command-name>/model</command-name>"))
		}
		var b strings.Builder
		for _, r := range recs {
			x, _ := json.Marshal(r)
			b.Write(x)
			b.WriteByte('\n')
		}
		p := filepath.Join(root, "claude", "-home-dev-trail-planner", sid+".jsonl")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// "Unused skills" end to end, within the budget and over it: the row and its fold in Look things up, the same
// in Markdown, and with the list over its budget, the fix's details and the terminal line. The first screen and
// the terminal are unchanged when the list is within its budget.
func TestUnusedSkillsReportGolden(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	for _, over := range []bool{false, true} {
		root := t.TempDir()
		idleLogs(t, root, over)
		if err := os.MkdirAll(filepath.Join(root, "codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		for k, v := range map[string]string{"LEEWAY_NOW": "2026-03-01T00:00:00Z", "LEEWAY_CLAUDE_DIR": filepath.Join(root, "claude"), "LEEWAY_CODEX_DIR": filepath.Join(root, "codex"),
			"LEEWAY_CONFIG": filepath.Join(root, "config"), "LEEWAY_CACHE": filepath.Join(root, "cache"), "CLAUDE_CONFIG_DIR": filepath.Join(root, "claude-home"),
			"CODEX_HOME": filepath.Join(root, "codex-home")} {
			t.Setenv(k, v)
		}
		state := map[bool]string{false: "within", true: "over"}[over]
		for _, lang := range []string{"en", "ko"} {
			dir := filepath.Join(root, "reports-"+lang)
			out, errs, rc := run(t, "report", "--lang", lang, "--out", dir, "--no-archived")
			if rc != 0 {
				t.Fatalf("report: %d %s", rc, errs)
			}
			files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
			if len(files) != 1 {
				t.Fatalf("files %v", files)
			}
			md, _ := os.ReadFile(files[0])
			h, _ := os.ReadFile(strings.TrimSuffix(files[0], ".md") + ".html")
			term := strings.ReplaceAll(out, dir+string(filepath.Separator), "<reports>/")
			name := "report-skills-" + state + "." + lang
			golden(t, name+".md", string(md))
			golden(t, name+".html", string(h))
			golden(t, "report-skills-"+state+"-terminal."+lang+".txt", term)
			plugin := map[string]string{"en": "atlas: none of this plugin's 3 skills was used in the last 29 days.", "ko": "atlas: 최근 29일 동안 이 플러그인의 스킬 3개를 하나도 쓰지 않았습니다."}[lang]
			count := map[string]string{"en": "5 of 10 not used in the last 29 days", "ko": "최근 29일 동안 10개 중 5개를 쓰지 않음"}[lang]
			for what, s := range map[string]string{"md": string(md), "html": html.UnescapeString(string(h))} {
				if !strings.Contains(s, plugin) || !strings.Contains(s, count) {
					t.Errorf("%s %s %s: no unused-skills row with its plugin line", state, lang, what)
				}
				if !strings.Contains(s, map[string]string{"en": "list seen in 22 sessions", "ko": "목록이 기록된 세션 22개"}[lang]) {
					t.Errorf("%s %s %s: the automation's list counted", state, lang, what)
				}
				if !strings.Contains(s, map[string]string{"en": "Other skills not used: gif-maker.", "ko": "그 밖에 쓰지 않은 스킬: gif-maker."}[lang]) {
					t.Errorf("%s %s %s: a skill only an automation used is called unused", state, lang, what)
				}
				if strings.Contains(s, "svg-icons, ") || strings.Contains(s, "pdf-forms,") || strings.Contains(s, "ledger:export,") {
					t.Errorf("%s %s %s: a used skill is listed as unused", state, lang, what)
				}
			}
			// automations' uses count for "used least first" too: release-notes, used only by the automation, comes last
			if over && !strings.Contains(string(md), map[string]string{"en": "used least first: gif-maker, atlas:tiles, release-notes.", "ko": "덜 쓴 순서: gif-maker, atlas:tiles, release-notes."}[lang]) {
				t.Errorf("%s md: used least first without the automation's use", lang)
			}
			if strings.Contains(term, plugin) != over {
				t.Errorf("%s %s terminal: the plugin line only with the fix\n%s", state, lang, term)
			}
		}
	}
}

// Made-up Codex rollouts for "Unused skills": 21 direct sessions with the same list of six skills (two from the
// atlas plugin), and one whose list was hidden by config. pdf-forms is picked, gif-maker's SKILL.md is read by a
// command, notes's folder is searched with rg (a call naming the folder counts); the atlas skills and deploy are never
// used. Three of six unused, half: a card.
func codexIdleLogs(t *testing.T, root string) {
	t.Helper()
	t0 := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	list := "<skills_instructions>\n## Skills\n### Skill roots\n- `r0` = `/home/dev/.agents/skills`\n- `r1` = `/home/dev/.codex/plugins/cache/mkt/atlas/1.2/skills`\n" + // privacy-ok: made-up path
		"### Available skills\n- pdf-forms: Fill PDF forms. (file: r0/pdf-forms/SKILL.md)\n- gif-maker: Make short GIFs. (file: r0/gif-maker/SKILL.md)\n" +
		"- notes: Keep meeting notes. (file: r0/notes/SKILL.md)\n- deploy: Ship the site. (file: r0/deploy/SKILL.md)\n" +
		"- atlas:maps: Draw maps. (file: r1/maps/SKILL.md)\n- atlas:routes: Plan routes. (file: r1/routes/SKILL.md)\n### How to use skills\n- Open the SKILL.md first.\n</skills_instructions>"
	for i := 0; i < 22; i++ {
		id := fmt.Sprintf("thread-idle-%02d", i)
		at := t0.Add(time.Duration(i) * 31 * time.Hour)
		ts := func(k int) string {
			return at.Add(time.Duration(k) * 20 * time.Second).Format("2006-01-02T15:04:05.000Z")
		}
		cwd := "/home/dev/invoice-sync" // privacy-ok: made-up path
		recs := []map[string]any{
			{"timestamp": ts(0), "type": "session_meta", "payload": map[string]any{"id": id, "cwd": cwd, "originator": "codex-tui", "source": "cli", "cli_version": "0.172.0"}},
			{"timestamp": ts(0), "type": "event_msg", "payload": map[string]any{"type": "task_started", "turn_attribution": map[string]any{"turn_trigger": "composer"}}},
			{"timestamp": ts(0), "type": "turn_context", "payload": map[string]any{"model": "gpt-5.4", "cwd": cwd}},
		}
		if i == 21 { // list hidden by config: flagged, not counted
			recs = append(recs, map[string]any{"timestamp": ts(1), "type": "world_state", "payload": map[string]any{"full": true, "state": map[string]any{
				"host_skills": map[string]any{"includeInstructions": false, "body": nil}}}})
		} else {
			recs = append(recs, map[string]any{"timestamp": ts(1), "type": "response_item", "payload": map[string]any{"type": "message", "role": "developer",
				"content": []any{map[string]any{"type": "input_text", "text": list}}}})
		}
		user := func(k int, text string) map[string]any {
			return map[string]any{"timestamp": ts(k), "type": "response_item", "payload": map[string]any{"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": text}}}}
		}
		command := func(k int, c string) map[string]any {
			return map[string]any{"timestamp": ts(k), "type": "event_msg", "payload": map[string]any{"type": "item_completed", "item": map[string]any{
				"type": "CommandExecution", "id": fmt.Sprintf("%s-c%d", id, k), "command": []any{"/bin/zsh", "-lc", c}, "cwd": cwd, "exit_code": 0}}}
		}
		recs = append(recs, user(2, "check the export page"))
		switch i {
		case 4:
			recs = append(recs, user(3, "<skill>\n<name>pdf-forms</name>\n<path>/home/dev/.agents/skills/pdf-forms/SKILL.md</path>\nFill the form.\n</skill>")) // privacy-ok: made-up path
		case 9:
			recs = append(recs, command(3, "cat /home/dev/.agents/skills/gif-maker/SKILL.md")) // privacy-ok: made-up path
		case 15:
			recs = append(recs, command(3, "rg -n meeting /home/dev/.agents/skills/notes/SKILL.md")) // privacy-ok: made-up path
		}
		for k := 4; k < 8; k++ {
			recs = append(recs, map[string]any{"timestamp": ts(k), "type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{
				"last_token_usage":  map[string]any{"input_tokens": 20000 + k, "cached_input_tokens": 18000, "output_tokens": 100},
				"total_token_usage": map[string]any{"input_tokens": 20000 * k}}}})
		}
		var b strings.Builder
		for _, r := range recs {
			x, _ := json.Marshal(r)
			b.Write(x)
			b.WriteByte('\n')
		}
		p := filepath.Join(root, "codex", at.Format("2006/01/02"), "rollout-"+at.Format("2006-01-02T15-04-05")+"-"+id+".jsonl")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Codex "Unused skills" end to end: the row, its fold, the card in Other things worth fixing (four of six unused)
// and the terminal line; picked and read skills are used, a skill only searched is not; the hidden list is noted.
// The card never enters the first three fixes or the rating.
func TestUnusedCodexSkillsReportGolden(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	root := t.TempDir()
	codexIdleLogs(t, root)
	if err := os.MkdirAll(filepath.Join(root, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"LEEWAY_NOW": "2026-03-01T00:00:00Z", "LEEWAY_CLAUDE_DIR": filepath.Join(root, "claude"), "LEEWAY_CODEX_DIR": filepath.Join(root, "codex"),
		"LEEWAY_CONFIG": filepath.Join(root, "config"), "LEEWAY_CACHE": filepath.Join(root, "cache"), "CLAUDE_CONFIG_DIR": filepath.Join(root, "claude-home"),
		"CODEX_HOME": filepath.Join(root, "codex-home")} {
		t.Setenv(k, v)
	}
	for _, lang := range []string{"en", "ko"} {
		dir := filepath.Join(root, "reports-"+lang)
		out, errs, rc := run(t, "report", "--lang", lang, "--out", dir, "--no-archived")
		if rc != 0 {
			t.Fatalf("report: %d %s", rc, errs)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		if len(files) != 1 {
			t.Fatalf("files %v", files)
		}
		md, _ := os.ReadFile(files[0])
		h, _ := os.ReadFile(strings.TrimSuffix(files[0], ".md") + ".html")
		term := strings.ReplaceAll(out, dir+string(filepath.Separator), "<reports>/")
		golden(t, "report-skills-codex."+lang+".md", string(md))
		golden(t, "report-skills-codex."+lang+".html", string(h))
		golden(t, "report-skills-codex-terminal."+lang+".txt", term)
		want := map[string][]string{
			"en": {"3 of 6 not used in the last 29 days", "atlas: none of this plugin's 2 skills was used in the last 29 days.", "Other skills not used: deploy.",
				"Not counted: 1 session whose list was hidden or cut short.", "Turn off the skills you do not use", "[[skills.config]]"},
			"ko": {"최근 29일 동안 6개 중 3개를 쓰지 않음", "atlas: 최근 29일 동안 이 플러그인의 스킬 2개를 하나도 쓰지 않았습니다.", "그 밖에 쓰지 않은 스킬: deploy.",
				"목록이 숨겨졌거나 잘린 세션 1개는 세지 않았습니다.", "안 쓰는 스킬은 끄세요", "[[skills.config]]"},
		}[lang]
		for what, s := range map[string]string{"md": string(md), "html": html.UnescapeString(string(h))} {
			for _, w := range want {
				if !strings.Contains(s, w) {
					t.Errorf("%s %s: missing %q", lang, what, w)
				}
			}
			if strings.Contains(s, "pdf-forms,") || strings.Contains(s, "gif-maker,") || strings.Contains(s, "notes.") {
				t.Errorf("%s %s: a used skill is listed as unused", lang, what)
			}
		}
		if !strings.Contains(term, map[string]string{"en": "Also worth a look: 3 of 6 Codex skills", "ko": "함께 볼 것: 최근 29일 동안 Codex 스킬 6개 중 3개"}[lang]) {
			t.Errorf("%s terminal: no line for the card\n%s", lang, term)
		}
		// the card is in Look things up, after every first-screen fix, and nowhere before it
		look, card := strings.Index(string(h), `id="s-look"`), strings.Index(string(h), `id="f-setup-idle-codex"`)
		if look < 0 || card < look || strings.Count(string(h), `ev-f-setup-idle-codex"`) != 3 {
			t.Errorf("%s: the card is not (only) in Look things up: look %d, card %d", lang, look, card)
		}
		if i := strings.Index(string(md), `<a id="f-setup-idle-codex">`); i < 0 || i < strings.Index(string(md), "## "+map[string]string{"en": "Look things up", "ko": "찾아보기"}[lang]) {
			t.Errorf("%s md: the card is not in Look things up", lang)
		}
	}
}
