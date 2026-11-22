// Package licenses builds THIRD_PARTY_LICENSES.txt: the license texts of everything compiled into
// agent-leeway (the Go standard library and each module), found from the build itself rather than by hand,
// so the file cannot drift from go.mod. Regenerate with `go run ./internal/licenses/gen`; a test fails when
// the committed file is stale.
package licenses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FileName is the generated file at the repository root (shipped in every release archive).
const FileName = "THIRD_PARTY_LICENSES.txt"

// Targets are the operating systems whose builds are inspected (package sets differ, e.g. x/sys/windows).
var Targets = []string{"darwin", "linux", "windows"}

// Module is one third-party module linked into the binary.
type Module struct {
	Path, Version, Dir string
	pkgDirs            map[string]bool
}

type listed struct {
	ImportPath string
	Dir        string
	Standard   bool
	Module     *struct {
		Path, Version, Dir string
		Main               bool
	}
}

// Linked lists the third-party modules (and their package folders) in the agent-leeway builds for Targets.
func Linked(root string) ([]*Module, error) {
	mods := map[string]*Module{}
	for _, goos := range Targets {
		cmd := exec.Command("go", "list", "-deps", "-json", "./cmd/agent-leeway")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list (%s): %v", goos, err)
		}
		dec := json.NewDecoder(bytes.NewReader(out))
		for dec.More() {
			var p listed
			if err := dec.Decode(&p); err != nil {
				return nil, err
			}
			if p.Standard || p.Module == nil || p.Module.Main {
				continue
			}
			m := mods[p.Module.Path]
			if m == nil {
				m = &Module{Path: p.Module.Path, Version: p.Module.Version, Dir: p.Module.Dir, pkgDirs: map[string]bool{}}
				mods[p.Module.Path] = m
			}
			m.pkgDirs[p.Dir] = true
		}
	}
	var list []*Module
	for _, m := range mods {
		list = append(list, m)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].Path < list[b].Path })
	return list, nil
}

func isLicenseName(n string) bool {
	u := strings.ToUpper(n)
	for _, p := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE", "PATENTS"} {
		if strings.HasPrefix(u, p) {
			return true
		}
	}
	return false
}

// licenseFiles are the license files at the module root and in every folder between it and a linked package.
func (m *Module) licenseFiles() []string {
	dirs := map[string]bool{m.Dir: true}
	for d := range m.pkgDirs {
		for p := d; strings.HasPrefix(p, m.Dir); p = filepath.Dir(p) {
			dirs[p] = true
			if p == m.Dir {
				break
			}
		}
	}
	var files []string
	for d := range dirs {
		ents, _ := os.ReadDir(d)
		for _, e := range ents {
			if !e.IsDir() && isLicenseName(e.Name()) {
				files = append(files, filepath.Join(d, e.Name()))
			}
		}
	}
	sort.Strings(files)
	return files
}

// goLicense finds the Go distribution's LICENSE and PATENTS (some packagings keep them next to GOROOT).
func goLicense() ([]string, error) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(string(out))
	var files []string
	for _, n := range []string{"LICENSE", "PATENTS"} {
		for _, d := range []string{root, filepath.Dir(root)} {
			if _, err := os.Stat(filepath.Join(d, n)); err == nil {
				files = append(files, filepath.Join(d, n))
				break
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("Go LICENSE not found near %s", root)
	}
	return files, nil
}

func normalize(b []byte) string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n"
}

// Generate returns the text of THIRD_PARTY_LICENSES.txt for the repository at root.
func Generate(root string) (string, error) {
	mods, err := Linked(root)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Third-party software in agent-leeway\n")
	b.WriteString("=====================================\n\n")
	b.WriteString("agent-leeway itself is under the MIT License (LICENSE). The executable also contains the code below;\n")
	b.WriteString("each part is distributed under its own license, reproduced in full. This file is generated from the\n")
	b.WriteString("build (go run ./internal/licenses/gen); do not edit it by hand.\n\n")
	gl, err := goLicense()
	if err != nil {
		return "", err
	}
	section := func(title string, files []string, base string) error {
		b.WriteString(strings.Repeat("-", 78) + "\n" + title + "\n" + strings.Repeat("-", 78) + "\n\n")
		for _, f := range files {
			t, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			rel := filepath.Base(f)
			if base != "" {
				if r, err := filepath.Rel(base, f); err == nil {
					rel = filepath.ToSlash(r)
				}
			}
			b.WriteString("[" + rel + "]\n\n" + normalize(t) + "\n")
		}
		return nil
	}
	if err := section("Go standard library and runtime (The Go Authors)", gl, ""); err != nil {
		return "", err
	}
	for _, m := range mods {
		files := m.licenseFiles()
		if len(files) == 0 {
			return "", fmt.Errorf("%s %s: no license file found", m.Path, m.Version)
		}
		if err := section(m.Path+" "+m.Version, files, m.Dir); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}
