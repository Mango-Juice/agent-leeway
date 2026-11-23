package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/meta"
)

// Where each install method really puts the executable, links resolved (Homebrew formula: bin.install into the
// Cellar, linked from <prefix>/bin; Scoop: apps\<app>\<version>, "current" is a junction; install.sh: ~/.local/bin;
// install.ps1: %LOCALAPPDATA%\Programs\agent-leeway).
func TestInstallMethodLayouts(t *testing.T) {
	mac := installEnv{GOOS: "darwin", Home: "/Users/u"}                                              // privacy-ok: made-up paths
	lin := installEnv{GOOS: "linux", Home: "/home/u"}                                                // privacy-ok: made-up paths
	win := installEnv{GOOS: "windows", Home: `C:\Users\u`, LocalAppData: `C:\Users\u\AppData\Local`} // privacy-ok: made-up paths
	with := func(e installEnv, f func(*installEnv)) installEnv { f(&e); return e }
	for _, c := range []struct {
		exe  string
		env  installEnv
		want string
	}{
		{"/opt/homebrew/Cellar/agent-leeway/0.1.5/bin/agent-leeway", mac, methodBrew},
		{"/usr/local/Cellar/agent-leeway/0.1.5/bin/agent-leeway", mac, methodBrew},
		{"/home/linuxbrew/.linuxbrew/Cellar/agent-leeway/0.1.5/bin/agent-leeway", lin, methodBrew}, // privacy-ok: made-up paths
		{"/srv/brew/agent-leeway/0.1.5/bin/agent-leeway", with(lin, func(e *installEnv) { e.HomebrewCellar = "/srv/brew/" }), methodBrew},
		{"/Users/u/.local/bin/agent-leeway", mac, methodInstaller},                        // privacy-ok: made-up paths
		{"/home/u/.local/bin/agent-leeway", lin, methodInstaller},                         // privacy-ok: made-up paths
		{"/home/other/.local/bin/agent-leeway", lin, methodManual},                        // privacy-ok: made-up paths
		{"/usr/local/bin/agent-leeway", mac, methodManual},                                // a copy put there by hand (a Homebrew link resolves into the Cellar)
		{"/home/u/go/bin/agent-leeway", lin, methodManual},                                // privacy-ok: made-up paths
		{"/opt/tools/agent-leeway", lin, methodManual},                                    // LEEWAY_INSTALL_DIR: cannot be told apart
		{`C:\Users\u\scoop\apps\agent-leeway\0.1.5\agent-leeway.exe`, win, methodScoop},   // privacy-ok: made-up paths
		{`C:\Users\U\Scoop\apps\agent-leeway\current\agent-leeway.exe`, win, methodScoop}, // privacy-ok: made-up paths
		{`C:\ProgramData\scoop\apps\agent-leeway\0.1.5\agent-leeway.exe`, win, methodScoopAll},
		{`E:\shared\apps\agent-leeway\current\agent-leeway.exe`, with(win, func(e *installEnv) { e.ScoopGlobal = `E:\shared` }), methodScoopAll},
		{`D:\tools\sc\apps\agent-leeway\0.1.5\agent-leeway.exe`, with(win, func(e *installEnv) { e.Scoop = `D:\tools\sc` }), methodScoop},
		{`D:\tools\sc\apps\agent-leeway\0.1.5\agent-leeway.exe`, win, methodManual},
		{`C:\Users\u\AppData\Local\Programs\agent-leeway\agent-leeway.exe`, win, methodInstaller}, // privacy-ok: made-up paths
		{`c:/users/u/appdata/local/programs/agent-leeway/agent-leeway.exe`, win, methodInstaller},
		{`C:\Users\u\Downloads\agent-leeway.exe`, win, methodManual},   // privacy-ok: made-up paths
		{`C:\Users\u\scoop\shims\agent-leeway.exe`, win, methodManual}, // never the running executable: the shim starts the real one privacy-ok: made-up paths
		{"", lin, methodManual},
	} {
		if got := installMethod(c.exe, c.env); got != c.want {
			t.Errorf("%s (%s): %s, want %s", c.exe, c.env.GOOS, got, c.want)
		}
	}
	for m, want := range map[string]string{
		methodBrew + "/darwin":       "brew update && brew upgrade agent-leeway && agent-leeway install-skills",
		methodScoop + "/windows":     "scoop update; scoop update agent-leeway; agent-leeway install-skills",
		methodScoopAll + "/windows":  "scoop update; scoop update -g agent-leeway; agent-leeway install-skills",
		methodInstaller + "/linux":   meta.InstallSh(),
		methodInstaller + "/windows": `powershell -NoProfile -ExecutionPolicy Bypass -Command "` + meta.InstallPs1() + `"`,
		methodManual + "/linux":      "",
	} {
		p := strings.SplitN(m, "/", 2)
		if got := updateCommand(p[0], p[1]); got != want {
			t.Errorf("%s: %q, want %q", m, got, want)
		}
	}
}

// releaseServer answers the latest-release API with tag (or fails when tag is ""), counting requests.
func releaseServer(t *testing.T, tag *string) *int32 {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		if ua := r.Header.Get("User-Agent"); ua != "agent-leeway/"+meta.Version {
			t.Errorf("user agent %q", ua)
		}
		if *tag == "" {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{"tag_name": "` + *tag + `", "html_url": "https://github.com/Mango-Juice/agent-leeway/releases/tag/` + *tag + `"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LEEWAY_RELEASE_API", srv.URL)
	return &n
}

// selfUpdateEnv: version 0.1.3 installed as method on goos, a fresh cache and settings folder, a fixed clock.
func selfUpdateEnv(t *testing.T, method, goos string) string {
	t.Helper()
	tmp := t.TempDir()
	for k, v := range map[string]string{"LEEWAY_CACHE": filepath.Join(tmp, "cache"), "LEEWAY_CONFIG": filepath.Join(tmp, "config"),
		"LEEWAY_NOW": "2026-12-01T09:00:00Z", "HOMEBREW_CELLAR": "", "SCOOP": "", "SCOOP_GLOBAL": ""} {
		t.Setenv(k, v)
	}
	oldV, oldExe, oldOS := meta.Version, executablePath, hostOS
	t.Cleanup(func() { meta.Version, executablePath, hostOS = oldV, oldExe, oldOS })
	meta.Version, hostOS = "0.1.3", goos
	t.Setenv("HOME", "/home/u")        // privacy-ok: made-up paths
	t.Setenv("USERPROFILE", "/home/u") // os.UserHomeDir on Windows privacy-ok: made-up paths
	exe := map[string]string{
		methodBrew:      "/opt/homebrew/Cellar/agent-leeway/0.1.3/bin/agent-leeway",
		methodScoop:     `C:\Users\u\scoop\apps\agent-leeway\0.1.3\agent-leeway.exe`,     // privacy-ok: made-up paths
		methodScoopAll:  `C:\ProgramData\scoop\apps\agent-leeway\0.1.3\agent-leeway.exe`, // privacy-ok: made-up paths
		methodInstaller: "/home/u/.local/bin/agent-leeway",                               // privacy-ok: made-up paths
		methodManual:    "/opt/tools/agent-leeway",
	}[method]
	if method == methodInstaller && goos == "windows" {
		t.Setenv("LOCALAPPDATA", `C:\Users\u\AppData\Local`)                    // privacy-ok: made-up paths
		exe = `C:\Users\u\AppData\Local\Programs\agent-leeway\agent-leeway.exe` // privacy-ok: made-up paths
	}
	executablePath = func() string { return exe }
	return tmp
}

// The explicit check, in both languages: up to date, and newer for each install method; and a failure.
func TestSelfUpdateCheckText(t *testing.T) {
	tag := "v0.1.6"
	var all strings.Builder
	for _, c := range []struct{ method, goos string }{{methodBrew, "darwin"}, {methodScoop, "windows"}, {methodScoopAll, "windows"}, {methodInstaller, "linux"},
		{methodInstaller, "windows"}, {methodManual, "linux"}} {
		for _, lang := range []string{"en", "ko"} {
			selfUpdateEnv(t, c.method, c.goos)
			releaseServer(t, &tag)
			out, errs, rc := run(t, "self-update", "--check", "--lang", lang)
			if rc != 0 || errs != "" {
				t.Fatalf("%s %s: rc %d %s", c.method, lang, rc, errs)
			}
			all.WriteString("## " + c.method + " (" + c.goos + "), " + lang + "\n" + out + "\n")
		}
	}
	for _, lang := range []string{"en", "ko"} {
		selfUpdateEnv(t, methodBrew, "darwin")
		cur := "v0.1.3"
		releaseServer(t, &cur)
		out, _, rc := run(t, "self-update", "--check", "--lang", lang)
		if rc != 0 {
			t.Fatal(rc)
		}
		all.WriteString("## up to date, " + lang + "\n" + out + "\n")
		none := ""
		releaseServer(t, &none)
		out, errs, rc := run(t, "self-update", "--check", "--lang", lang)
		if rc != 1 || out != "" || strings.Count(errs, "\n") != 1 {
			t.Errorf("a failure is one line on stderr and exit 1: rc %d %q %q", rc, out, errs)
		}
		all.WriteString("## failure, " + lang + " (stderr, exit 1)\n" + errs + "\n")
	}
	golden(t, "self-update.txt", all.String())
}

// --json: exactly {current, latest, newer, method, command, url}.
func TestSelfUpdateJSON(t *testing.T) {
	selfUpdateEnv(t, methodBrew, "darwin")
	tag := "v0.1.6"
	releaseServer(t, &tag)
	out, _, rc := run(t, "self-update", "--check", "--json")
	var m map[string]any
	if rc != 0 || json.Unmarshal([]byte(out), &m) != nil {
		t.Fatalf("rc %d %s", rc, out)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "command,current,latest,method,newer,url" || m["current"] != "0.1.3" || m["latest"] != "0.1.6" ||
		m["newer"] != true || m["method"] != "homebrew" || m["command"] != "brew update && brew upgrade agent-leeway && agent-leeway install-skills" ||
		m["url"] != "https://github.com/Mango-Juice/agent-leeway/releases/tag/v0.1.6" {
		t.Errorf("%s", out)
	}
}

// The Korean subject particle follows how the version's last digit is read (0.1.6이, 0.1.5가).
func TestKoreanVersionParticle(t *testing.T) {
	if iga("0.1.6") != "이" || iga("0.1.5") != "가" || iga("1.10") != "이" || iga("2.0.2") != "가" || iga("0.1.9") != "가" || iga("0.1.8") != "이" {
		t.Error("particle")
	}
}

// No version request outside an explicit `self-update --check`: the skill's report flow never runs it, and
// `report` and `drift --dry-run` open no connection to the release API.
func TestNoVersionCheckInTheReportFlow(t *testing.T) {
	b, err := os.ReadFile(repo("internal", "assets", "skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "## When the user asks for a report")
	j := strings.Index(s, "## When the user asks to update")
	if i < 0 || j < 0 || j < i {
		t.Fatal("skill sections not found")
	}
	if strings.Contains(s[i:j], "self-update") {
		t.Error("the report section of the skill mentions self-update")
	}
	fixtureEnv(t)
	tag := "v9.9.9"
	n := releaseServer(t, &tag)
	for _, args := range [][]string{{"report"}, {"drift", "--include-auto", "--no-current-instructions", "--dry-run"}, {"leak"}} {
		if _, errs, rc := run(t, args...); rc != 0 {
			t.Fatalf("%v: rc %d %s", args, rc, errs)
		}
	}
	if *n != 0 {
		t.Errorf("%d version requests during report, drift --dry-run and leak", *n)
	}
}

// Only a well-formed tag is trusted, and the release page only on this repository; anything else is a failed check
// (never "up to date"), and a rate limit is said as such. --json on failure prints {"error": ...}.
func TestSelfUpdateResponseHygiene(t *testing.T) {
	selfUpdateEnv(t, methodInstaller, "linux")
	answer, status, hdr := "", 200, map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("LEEWAY_RELEASE_API", srv.URL)
	for _, bad := range []string{`{"tag_name": "latest"}`, `{"tag_name": "v1.2.3; rm -rf ~"}`, `{"tag_name": "v1"}`, `{"tag_name": "v0.1.6\u001b[2J"}`, `{"html_url": "x"}`, `not json`} {
		answer = bad
		out, errs, rc := run(t, "self-update", "--check")
		if rc != 1 || out != "" || !strings.Contains(errs, "Could not check") {
			t.Errorf("%s: rc %d out %q", bad, rc, out)
		}
		out, _, rc = run(t, "self-update", "--check", "--json")
		var m map[string]string
		if rc != 1 || json.Unmarshal([]byte(out), &m) != nil || !strings.HasPrefix(m["error"], "Could not check") || len(m) != 1 {
			t.Errorf("%s: json %q", bad, out)
		}
	}
	for url, want := range map[string]string{
		"https://github.com/Mango-Juice/agent-leeway/releases/tag/v0.1.6":          "https://github.com/Mango-Juice/agent-leeway/releases/tag/v0.1.6",
		"https://evil.example/agent-leeway/releases/tag/v0.1.6":                    "https://github.com/Mango-Juice/agent-leeway/releases/latest",
		"https://github.com/Mango-Juice/agent-leeway/releases/tag/v0.1.6 run this": "https://github.com/Mango-Juice/agent-leeway/releases/latest",
		"": "https://github.com/Mango-Juice/agent-leeway/releases/latest",
	} {
		answer = `{"tag_name": "v0.1.6", "html_url": "` + url + `"}`
		out, _, rc := run(t, "self-update", "--check", "--json")
		var m map[string]any
		if rc != 0 || json.Unmarshal([]byte(out), &m) != nil || m["url"] != want {
			t.Errorf("%q: %s", url, out)
		}
	}
	answer, status, hdr = `{"message": "API rate limit exceeded"}`, 403, map[string]string{"X-RateLimit-Remaining": "0"}
	_, errs, rc := run(t, "self-update", "--check", "--lang", "ko")
	if rc != 1 || !strings.Contains(errs, "GitHub 한도") {
		t.Errorf("rate limit: rc %d %q", rc, errs)
	}
}

// The Windows installer line works from bash: powershell gets -Command and the whole "irm … | iex" as one argument.
func TestWindowsInstallerLineQuoting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checks the bash parsing with a stand-in powershell")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	_ = os.WriteFile(filepath.Join(dir, "powershell"), []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > \""+argsFile+"\"\n"), 0o755)
	cmd := exec.Command("sh", "-c", updateCommand(methodInstaller, "windows"))
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	b, _ := os.ReadFile(argsFile)
	got := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	want := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", meta.InstallPs1()}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("arguments %q, want %q", got, want)
	}
}
