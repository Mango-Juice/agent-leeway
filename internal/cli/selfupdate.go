package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/meta"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// self-update --check says whether a newer release exists and prints the one command that updates this copy, chosen
// from where the running executable really is (Homebrew, Scoop, the one-line installer, or a manual download). It
// downloads nothing and never replaces the executable. It runs only when the user asks; no other command checks.

// Install methods.
const (
	methodBrew      = "homebrew"
	methodScoop     = "scoop"
	methodScoopAll  = "scoop-global" // installed for all users (scoop install -g): updating needs an administrator shell
	methodInstaller = "installer"
	methodManual    = "manual"
)

// executablePath is the running executable with symbolic links (and Windows junctions) resolved; tests replace it.
var executablePath = func() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// hostOS picks the installer line (install.sh or install.ps1); tests replace it.
var hostOS = runtime.GOOS

// installEnv is what installMethod reads besides the path; tests set it per OS.
type installEnv struct {
	GOOS                               string
	Home, LocalAppData                 string // home folder; %LOCALAPPDATA% on Windows
	HomebrewCellar, Scoop, ScoopGlobal string // $HOMEBREW_CELLAR, $SCOOP, $SCOOP_GLOBAL when set
}

func currentInstallEnv() installEnv {
	h, _ := os.UserHomeDir()
	return installEnv{GOOS: hostOS, Home: h, LocalAppData: os.Getenv("LOCALAPPDATA"),
		HomebrewCellar: os.Getenv("HOMEBREW_CELLAR"), Scoop: os.Getenv("SCOOP"), ScoopGlobal: os.Getenv("SCOOP_GLOBAL")}
}

// installMethod tells how the executable at exe (links resolved) was installed:
//   - Homebrew keeps it in <prefix>/Cellar/agent-leeway/<version>/bin/ (/opt/homebrew, /usr/local, or
//     /home/linuxbrew/.linuxbrew), linked from <prefix>/bin; privacy-ok: made-up paths
//   - Scoop keeps it in <scoop>\apps\agent-leeway\<version>\ (\current\ is a junction to it), started through a shim in
//     <scoop>\shims; <scoop> is %USERPROFILE%\scoop or $SCOOP, and $SCOOP_GLOBAL (%ProgramData%\scoop) for an install
//     for all users (scoop-global);
//   - the one-line installer puts it in ~/.local/bin (install.sh) or %LOCALAPPDATA%\Programs\agent-leeway
//     (install.ps1); a copy installed elsewhere with LEEWAY_INSTALL_DIR cannot be told from a manual one.
func installMethod(exe string, e installEnv) string {
	if exe == "" {
		return methodManual
	}
	if e.GOOS == "windows" {
		norm := func(p string) string { return strings.TrimRight(strings.ToLower(strings.ReplaceAll(p, "/", `\`)), `\`) }
		p, dir := norm(exe), ""
		if i := strings.LastIndex(p, `\`); i >= 0 {
			dir = p[:i]
		}
		if (e.ScoopGlobal != "" && strings.HasPrefix(p, norm(e.ScoopGlobal)+`\apps\agent-leeway\`)) || strings.Contains(p, `\programdata\scoop\apps\agent-leeway\`) {
			return methodScoopAll
		}
		if (e.Scoop != "" && strings.HasPrefix(p, norm(e.Scoop)+`\apps\agent-leeway\`)) || strings.Contains(p, `\scoop\apps\agent-leeway\`) {
			return methodScoop
		}
		if e.LocalAppData != "" && dir == norm(e.LocalAppData)+`\programs\agent-leeway` {
			return methodInstaller
		}
		return methodManual
	}
	if strings.Contains(exe, "/Cellar/agent-leeway/") || (e.HomebrewCellar != "" && strings.HasPrefix(exe, strings.TrimRight(e.HomebrewCellar, "/")+"/agent-leeway/")) {
		return methodBrew
	}
	if e.Home != "" && filepath.ToSlash(pathDir(exe)) == strings.TrimRight(filepath.ToSlash(e.Home), "/")+"/.local/bin" {
		return methodInstaller
	}
	return methodManual
}

// pathDir is the folder of a slash-separated path (filepath.Dir would use the host's separator).
func pathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return ""
}

// updateCommand is the one command that updates a copy installed this way ("" for a manual download).
//   - Homebrew and Scoop refresh their taps and buckets only now and then (Homebrew once a day by default,
//     HOMEBREW_AUTO_UPDATE_SECS; Scoop when its last update is 3 hours old), so the command refreshes them first.
//   - They do not touch the skills, so the command refreshes those too, as the installer does.
//   - The Windows installer line runs through powershell.exe, so it works from bash, cmd and PowerShell alike.
func updateCommand(method, goos string) string {
	skills := config.Name + " install-skills"
	switch method {
	case methodBrew:
		return "brew update && brew upgrade agent-leeway && " + skills
	case methodScoop:
		return "scoop update; scoop update agent-leeway; " + skills
	case methodScoopAll:
		return "scoop update; scoop update -g agent-leeway; " + skills
	case methodInstaller:
		if goos == "windows" {
			return `powershell -NoProfile -ExecutionPolicy Bypass -Command "` + meta.InstallPs1() + `"`
		}
		return meta.InstallSh()
	}
	return ""
}

// tagRE is what a release tag looks like; anything else in the answer is not trusted.
var tagRE = regexp.MustCompile(`^v?\d+(\.\d+){1,3}(-[0-9A-Za-z.]+)?$`)

// errRateLimit is GitHub saying the hourly limit for requests without a login is used up.
var errRateLimit = errors.New("rate limit")

// updateInfo is the answer, also printed by --json.
type updateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Newer   bool   `json:"newer"`
	Method  string `json:"method"`
	Command string `json:"command"`
	URL     string `json:"url"`
}

// latestRelease asks GitHub (or LEEWAY_RELEASE_API) for the latest release tag and page.
func latestRelease(timeout time.Duration) (tag, url string, err error) {
	api := os.Getenv("LEEWAY_RELEASE_API")
	if api == "" {
		api = meta.LatestReleaseAPI()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", config.Name+"/"+meta.Version) // no personal data
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if (resp.StatusCode == 403 || resp.StatusCode == 429) && (resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == 429) {
		return "", "", errRateLimit
	}
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel) != nil || !tagRE.MatchString(rel.TagName) {
		return "", "", fmt.Errorf("no valid release tag in the answer")
	}
	// the release page only when it is this repository's own; else the list of releases
	url = meta.ReleasesURL() + "/latest"
	if strings.HasPrefix(rel.HTMLURL, meta.ReleasesURL()+"/tag/") && printable(rel.HTMLURL) == rel.HTMLURL && !strings.ContainsAny(rel.HTMLURL, " \t\r\n\"'<>") {
		url = rel.HTMLURL
	}
	return rel.TagName, url, nil
}

func cmdSelfUpdate(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("self-update", flag.ContinueOnError)
	fs.SetOutput(errw)
	check := fs.Bool("check", false, "look up the latest release and print the one command that updates this copy (downloads nothing)")
	asJSON := fs.Bool("json", false, "print {current, latest, newer, method, command, url} as JSON")
	lang := fs.String("lang", "", "language: en or ko (default: settings, then the system language)")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	updLang = config.Language(*lang)
	messages.SetLang(updLang)
	T := messages.Cli
	if !*check {
		fmt.Fprintln(errw, py.Format(T["upd_no_replace"], map[string]any{"name": config.Name}))
		return 2
	}
	tag, url, err := latestRelease(10 * time.Second)
	if err != nil {
		why := err.Error()
		if errors.Is(err, errRateLimit) {
			why = T["upd_rate_limit"]
		}
		msg := py.Format(T["upd_failed"], map[string]any{"err": why})
		if *asJSON { // a script gets {"error": ...} and the exit code
			printOut(out, jsonLine(map[string]string{"error": msg}))
		}
		fmt.Fprintln(errw, msg)
		return 1
	}
	info := makeInfo(tag, url)
	if *asJSON {
		printOut(out, jsonLine(info))
		return 0
	}
	fmt.Fprint(out, updateText(info))
	return 0
}

func makeInfo(tag, url string) updateInfo {
	m := installMethod(executablePath(), currentInstallEnv())
	i := updateInfo{Current: meta.Version, Latest: strings.TrimPrefix(tag, "v"), Newer: newer(tag, meta.Version), Method: m,
		Command: updateCommand(m, hostOS), URL: url}
	if m == methodManual {
		i.URL = meta.ReleasesURL() + "/latest"
	}
	return i
}

func jsonLine(v any) string {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false) // "&&" stays as it is in the command
	_ = e.Encode(v)
	return strings.TrimRight(b.String(), "\n")
}

// updateText is the explicit check's answer: up to date, or the newer version and the one command for this copy.
func updateText(i updateInfo) string {
	T := messages.Cli
	f := func(k string) string {
		return py.Format(T[k], map[string]any{"name": config.Name, "cur": i.Current, "v": i.Latest, "vs": subj(i.Latest), "url": i.URL})
	}
	if !i.Newer {
		return f("upd_current") + "\n"
	}
	var b strings.Builder
	if i.Method == methodManual { // the one link is the download page
		b.WriteString(f("upd_newer_short") + "\n" + f("upd_manual") + "\n  " + i.URL + "\n")
		return b.String()
	}
	b.WriteString(f("upd_newer") + "\n")
	switch i.Method {
	case methodInstaller:
		b.WriteString(f("upd_run_installer") + "\n  " + i.Command + "\n")
	case methodScoopAll:
		b.WriteString(f("upd_run_admin") + "\n  " + i.Command + "\n" + f("upd_lag_scoop") + "\n")
	default:
		b.WriteString(f("upd_run_pkg") + "\n  " + i.Command + "\n" + f("upd_lag_"+i.Method) + "\n")
	}
	return b.String()
}

// updLang is the language of this run's messages.
var updLang = "en"

// subj is a version as the subject of a sentence: in Korean with its particle (0.1.6이, 0.1.5가).
func subj(v string) string {
	if updLang == "ko" {
		return v + iga(v)
	}
	return v
}

// iga is the Korean subject particle after a version number, by how its last digit is read (일, 이, 삼, ...):
// 0.1.6이, 0.1.5가.
func iga(v string) string {
	if v == "" {
		return "이"
	}
	switch v[len(v)-1] {
	case '2', '4', '5', '9':
		return "가"
	}
	return "이"
}
