package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The committed Formula/ and bucket/ files are exactly what the generator writes for the release they name, from
// that release's published checksums (testdata/packaging/checksums-v<version>.txt, downloaded from the release and
// verified). The packages workflow commits the three files together, so this holds after each release.
func TestCommittedPackagesMatchTheRelease(t *testing.T) {
	mb, err := os.ReadFile(filepath.Join("..", "..", "bucket", "agent-leeway.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cur struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(mb, &cur); err != nil || cur.Version == "" {
		t.Fatalf("bucket/agent-leeway.json has no version: %v", err)
	}
	v := cur.Version
	sumsFile := "testdata/packaging/checksums-v" + v + ".txt"
	b, err := os.ReadFile(filepath.Join("..", "..", sumsFile))
	if err != nil {
		t.Fatalf("%v (the packages workflow commits it with the formula and the manifest)", err)
	}
	sums, err := ParseChecksums(string(b))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"Formula/agent-leeway.rb": Formula(v, sums), "bucket/agent-leeway.json": Manifest(v, sums)} {
		got, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s differs from the generator output; run go run ./cmd/genpackages -version v%s -checksums %s", path, v, sumsFile)
		}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(Manifest(v, sums)), &m); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	f := Formula(v, sums)
	for _, h := range sums {
		if !strings.Contains(f, h) && !strings.Contains(Manifest(v, sums), h) {
			t.Errorf("hash %s used nowhere", h)
		}
	}
}

func TestChecksumsMustBeComplete(t *testing.T) {
	if _, err := ParseChecksums("b48017ccc107418fce85ddcb08a904a8dc9544fd094747b4e26fc5058a6c4786  agent-leeway_darwin_amd64.tar.gz\n"); err == nil {
		t.Error("missing archives must fail")
	}
	if _, err := ParseChecksums("nothex  agent-leeway_darwin_amd64.tar.gz\n"); err == nil {
		t.Error("a bad hash must fail")
	}
}

// packages.yml uses only the workflow's own token with contents: write, commits as github-actions[bot], checks
// the downloaded archives against checksums.txt and makes no commit when nothing changed (a re-run is a no-op).
func TestPackagesWorkflow(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "packages.yml"))
	if err != nil {
		t.Fatal(err)
	}
	w := string(b)
	for _, s := range []string{"types: [published]", "permissions: {}", "contents: write", "sha256sum -c checksums.txt", "go run ./cmd/genpackages",
		`git config user.name "github-actions[bot]"`, "41898282+github-actions[bot]@users.noreply.github.com", "if git diff --cached --quiet; then"} {
		if !strings.Contains(w, s) {
			t.Errorf("packages.yml lacks %q", s)
		}
	}
	if strings.Count(w, "permissions:") != 2 || strings.Contains(w, "id-token") || strings.Contains(w, "pull-requests") {
		t.Error("packages.yml asks for more permissions than contents: write")
	}
	if strings.Contains(strings.ReplaceAll(w, "github.token", ""), "secrets.") {
		t.Error("packages.yml must not use any secret besides the workflow's own token")
	}
}
