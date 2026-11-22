package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

func origins(t *testing.T, files ...*logs.File) map[string]SessionOrigin {
	tb := FromFiles(files)
	tb.Build()
	m := map[string]SessionOrigin{}
	for _, s := range tb.SessOrigin {
		m[s.Key] = s
	}
	return m
}

func sess(tool, id string, th logs.Thread, prompt string) *logs.File {
	th.Tool, th.ThreadID, th.SessionID, th.Source = tool, id, logs.Some(id), logs.Some("user")
	f := &logs.File{Thread: th, Steps: []logs.Step{{TS: at(0), Model: logs.Some("claude-opus-5-5"), Uncached: 10}}}
	if prompt != "" {
		f.Turns = []logs.Turn{{TS: at(0), Kind: "human", Text: prompt}}
	}
	return f
}

// docs (task 1): origin comes from what the log records about the launcher, not from one person's setup.
func TestOriginFromLauncherFields(t *testing.T) {
	m := origins(t,
		sess("claude", "typed", logs.Thread{Entrypoint: logs.Some("cli"), TypedPrompts: 2}, "hi"),
		sess("claude", "desktop", logs.Thread{Entrypoint: logs.Some("claude-desktop")}, "hi"),
		sess("claude", "headless", logs.Thread{Entrypoint: logs.Some("sdk-cli")}, "run the nightly check"),
		sess("claude", "sdk", logs.Thread{Entrypoint: logs.Some("sdk-ts")}, "hi"),
		sess("claude", "none", logs.Thread{}, "hi"),
		sess("claude", "worker", logs.Thread{Entrypoint: logs.Some("sdk-ts")}, "You are a dispatched worker. Do X."),
		sess("codex", "exec", logs.Thread{Originator: logs.Some("codex_exec")}, "x"),
		sess("codex", "execsrc", logs.Thread{Originator: logs.Some("my-wrapper"), LaunchSource: logs.Some("exec")}, "x"),
		sess("codex", "tui", logs.Thread{Originator: logs.Some("codex-tui")}, "x"),
		sess("codex", "thirdparty", logs.Thread{Originator: logs.Some("some_app")}, "x"),
	)
	want := map[string][3]string{ // class, reason, uncertain
		"typed": {"human", "interactive", ""}, "desktop": {"human", "interactive", ""}, "headless": {"auto", "auto:headless", ""},
		"sdk": {"human", "unknown_launcher", "u"}, "none": {"human", "no_launcher", "u"}, "worker": {"auto", "auto:worker_marker", ""},
		"exec": {"auto", "auto:codex_exec", ""}, "execsrc": {"auto", "auto:codex_exec", ""}, "tui": {"human", "interactive", ""},
		"thirdparty": {"human", "unknown_launcher", "u"},
	}
	for id, w := range want {
		tool := "claude"
		if id == "exec" || id == "execsrc" || id == "tui" || id == "thirdparty" {
			tool = "codex"
		}
		s := m[tool+":"+id]
		if s.SessionClass != w[0] || s.SessionReason != w[1] || s.Uncertain != (w[2] == "u") {
			t.Errorf("%s: %+v, want %v", id, s, w)
		}
	}
}

// Other orchestrators are added in settings.json, without a code change.
func TestOriginMarkersFromSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_CONFIG", dir)
	_ = os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"origin": {"worker_markers": ["[bot-runner]"], "interactive_entrypoints": ["cli", "my-ide"]}}`), 0o600)
	m := origins(t,
		sess("claude", "bot", logs.Thread{Entrypoint: logs.Some("sdk-ts")}, "[bot-runner] fix the build"),
		sess("claude", "ide", logs.Thread{Entrypoint: logs.Some("my-ide")}, "hi"),
		sess("claude", "old", logs.Thread{Entrypoint: logs.Some("sdk-ts")}, "You are a dispatched worker"),
	)
	if m["claude:bot"].SessionClass != "auto" || m["claude:ide"].Uncertain || m["claude:old"].SessionClass == "auto" {
		t.Errorf("settings not applied: %+v", m)
	}
}
