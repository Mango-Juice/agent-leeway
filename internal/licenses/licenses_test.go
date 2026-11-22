package licenses

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func root() string { r, _ := filepath.Abs(filepath.Join("..", "..")); return r }

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root(), p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The committed file equals what the current go.mod builds link.
func TestThirdPartyLicensesUpToDate(t *testing.T) {
	want, err := Generate(root())
	if err != nil {
		t.Fatal(err)
	}
	if read(t, FileName) != want {
		t.Fatalf("%s is stale: run `go run ./internal/licenses/gen`", FileName)
	}
}

// NOTICE names every linked module at the version in go.mod.
func TestNoticeListsLinkedModules(t *testing.T) {
	var notice string
	for _, p := range []string{"NOTICE.md", filepath.Join("public", "NOTICE.md")} {
		if b, err := os.ReadFile(filepath.Join(root(), p)); err == nil {
			notice = string(b)
			break
		}
	}
	if notice == "" {
		t.Fatal("no NOTICE.md")
	}
	mods, err := Linked(root())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mods {
		if strings.Contains(notice, "`"+m.Path+"` "+m.Version) {
			continue
		}
		if noticePending[m.Path] == m.Version && strings.Contains(notice, "`"+m.Path+"` ") {
			t.Logf("NOTICE.md still names an older %s; the docs owner updates it to %s (public/ is theirs)", m.Path, m.Version)
			continue
		}
		t.Errorf("NOTICE.md does not list `%s` %s", m.Path, m.Version)
	}
	if strings.Contains(notice, "Before the first release, add this NOTICE") {
		t.Error("NOTICE.md still says the archives lack it")
	}
}

// Every release archive carries the license files, and the installers keep them.
func TestReleaseShipsLicenseFiles(t *testing.T) {
	gr := read(t, ".goreleaser.yaml")
	m := regexp.MustCompile(`(?m)^\s+files: \[(.*)\]`).FindStringSubmatch(gr)
	if m == nil {
		t.Fatal("archives files list not found")
	}
	for _, f := range []string{"LICENSE*", "NOTICE*", FileName} {
		if !strings.Contains(m[1], f) {
			t.Errorf(".goreleaser.yaml archives do not include %s", f)
		}
	}
	for _, f := range []string{"LICENSE", "NOTICE.md", FileName} {
		for _, s := range []string{"install/install.sh", "install/install.ps1"} {
			if !strings.Contains(read(t, s), f) {
				t.Errorf("%s does not install %s", s, f)
			}
		}
		if !strings.Contains(gr, `"`+f+`"`) {
			t.Errorf("Homebrew formula does not install %s", f)
		}
	}
	// The allowlist exists only in the private research repository; the exported tree has no scripts/ folder.
	if fileExists(filepath.Join(root(), "scripts", "public_allowlist.txt")) && !strings.Contains(read(t, filepath.Join("scripts", "public_allowlist.txt")), FileName) {
		t.Errorf("public export does not include %s", FileName)
	}
}

// Tap and bucket commits use a bot identity, never the owner's.
func TestPublisherCommitsUseBotIdentity(t *testing.T) {
	gr := read(t, ".goreleaser.yaml")
	authors := regexp.MustCompile(`(?m)commit_author:\s*\n\s+name: (.+)\n\s+email: (.+)`).FindAllStringSubmatch(gr, -1)
	if len(authors) != 2 {
		t.Fatalf("expected commit_author on both publishers, found %d", len(authors))
	}
	for _, a := range authors {
		if !strings.Contains(a[1], "[bot]") || !strings.HasSuffix(strings.TrimSpace(a[2]), "[bot]@users.noreply.github.com") {
			t.Errorf("commit author %q <%s> is not a bot identity", a[1], a[2])
		}
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// The release workflow keeps secrets and the OIDC token away from tests and project code.
func TestReleaseWorkflowAttests(t *testing.T) {
	w := read(t, filepath.Join(".github", "workflows", "release.yml"))
	for _, s := range []string{"tags: [\"v*\"]", "permissions: {}", "goreleaser/goreleaser-action@", `version: "v2.20.3"`, "actions/attest@",
		"subject-checksums: checksums.txt", "git diff --exit-code " + FileName, "needs: test", "needs: build"} {
		if !strings.Contains(w, s) {
			t.Errorf("release.yml lacks %q", s)
		}
	}
	if strings.Contains(w, "TAP_GITHUB_TOKEN") {
		t.Error("the release job must not hold the tap token (formula and manifest are published separately)")
	}
	jobs := strings.Split(w, "\n  test:")[1]
	test := strings.Split(jobs, "\n  build:")[0]
	if strings.Contains(test, "secrets.") || strings.Contains(test, "write") {
		t.Error("the test job must have no secrets and no write permission")
	}
	build := strings.Split(strings.Split(jobs, "\n  build:")[1], "\n  attest:")[0]
	if strings.Contains(build, "id-token") {
		t.Error("only the attest job may request OIDC tokens")
	}
	if regexp.MustCompile(`(?m)^\s+(packages|artifact-metadata): write`).MatchString(w) {
		t.Error("release.yml asks for more permissions than it needs")
	}
	gr := read(t, ".goreleaser.yaml")
	if strings.Count(gr, "skip_upload: true") != 2 || strings.Contains(gr, "go test") {
		t.Error("goreleaser: tap and bucket must not be pushed from the release run; tests do not run inside it")
	}
}

// Every archive file and every file the Homebrew formula installs exists at the root of the exported tree
// (the release is built from it; NOTICE.md is public/NOTICE.md in this repository).
func TestArchiveFilesExistInExportedTree(t *testing.T) {
	top := map[string]bool{}
	if !fileExists(filepath.Join(root(), "scripts", "public_allowlist.txt")) { // already the exported tree
		ents, _ := os.ReadDir(root())
		for _, e := range ents {
			top[e.Name()] = true
		}
	}
	allow := ""
	if fileExists(filepath.Join(root(), "scripts", "public_allowlist.txt")) {
		allow = read(t, filepath.Join("scripts", "public_allowlist.txt"))
	}
	for _, line := range strings.Split(allow, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "?"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		dst := line
		if _, d, ok := strings.Cut(line, "->"); ok {
			dst = strings.TrimSpace(d)
		}
		top[strings.Split(dst, "/")[0]] = true
	}
	gr := read(t, ".goreleaser.yaml")
	files := regexp.MustCompile(`(?m)^\s+files: \[(.*)\]`).FindStringSubmatch(gr)[1]
	for _, g := range strings.Split(files, ",") {
		g = strings.TrimSpace(g)
		ok := false
		for name := range top {
			if m, _ := filepath.Match(g, name); m {
				ok = true
			}
		}
		if !ok {
			t.Errorf("archive glob %s matches nothing in the exported tree", g)
		}
	}
	for _, f := range []string{"LICENSE", "NOTICE.md", FileName} {
		if !top[f] {
			t.Errorf("%s is not at the root of the exported tree", f)
		}
	}
}

// noticePending lists dependency upgrades whose new version public/NOTICE.md (owned by the docs owner) does not
// show yet. Remove an entry once NOTICE.md is updated.
var noticePending = map[string]string{}

// Every action in the workflows is pinned to a full commit SHA, with the version kept in a comment.
func TestWorkflowActionsPinnedToSHA(t *testing.T) {
	pinned := regexp.MustCompile(`^[\w.-]+/[\w./-]+@[0-9a-f]{40} # v\d+(\.\d+)*$`)
	for _, f := range []string{"ci.yml", "release.yml"} {
		w := read(t, filepath.Join(".github", "workflows", f))
		for _, m := range regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*(.+?)\s*$`).FindAllStringSubmatch(w, -1) {
			if !pinned.MatchString(m[1]) {
				t.Errorf("%s: action not pinned to a commit SHA with a version comment: %s", f, m[1])
			}
		}
	}
}
