package logs

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Source is one log file to read.
type Source struct {
	Tool string // "codex" or "claude"
	Path string
}

// Discovery settings. The reference reads <codex sessions>/**/*.jsonl(.zst) and <claude projects>/**/*.jsonl.
type Discovery struct {
	CodexDirs  []string // sessions first; later folders (archived_sessions) only add files not already seen
	ClaudeDir  string
	SinceEpoch float64 // only files modified at or after this time (seconds); 0 = all
}

// Folder says what was found in one log folder, so an empty result can be told apart from "nothing leaked".
type Folder struct {
	Tool           string
	Dir            string
	Err            error // the folder is missing or cannot be read
	Unreadable     int   // subfolders that could not be read
	Files          int   // session files found
	InWindow       int   // of those, modified in the window
	Newest, Oldest int64 // modification times (Unix ns) of the newest and oldest file; 0 when none
}

// Discover lists log files the way the reference does: recursive, hidden names skipped (as Python's glob
// does), sorted by path, Codex before Claude, a compressed rollout only when its plain twin is absent.
func Discover(d Discovery) []Source {
	s, _ := DiscoverReport(d)
	return s
}

// DiscoverReport is Discover plus what was found in each folder.
func DiscoverReport(d Discovery) ([]Source, []Folder) {
	var out []Source
	var codex []string
	var folders []Folder
	codexFolder := map[string]int{}
	seen := map[string]bool{} // file names already taken from an earlier Codex folder
	for i, dir := range d.CodexDirs {
		files, unread, err := walk(dir)
		folders = append(folders, Folder{Tool: "codex", Dir: dir, Err: err, Unreadable: unread})
		plain := map[string]bool{}
		for _, p := range files {
			if strings.HasSuffix(p, ".jsonl") {
				plain[fold(p)] = true
			}
		}
		var mine []string
		for _, p := range files {
			if strings.HasSuffix(p, ".jsonl.zst") && plain[fold(strings.TrimSuffix(p, ".zst"))] {
				continue // both forms exist: the plain one is read
			}
			mine = append(mine, p)
		}
		sort.Strings(mine)
		for _, p := range mine {
			name := fold(strings.TrimSuffix(filepath.Base(p), ".zst"))
			if i > 0 && seen[name] {
				continue
			}
			seen[name] = true
			codex = append(codex, p)
			codexFolder[p] = len(folders) - 1
		}
	}
	claude, unread, err := walk(d.ClaudeDir)
	folders = append(folders, Folder{Tool: "claude", Dir: d.ClaudeDir, Err: err, Unreadable: unread})
	ci := len(folders) - 1
	var cl []string
	for _, p := range claude {
		if strings.HasSuffix(p, ".jsonl") {
			cl = append(cl, p)
		}
	}
	sort.Strings(cl)
	keep := func(p string, f *Folder) bool {
		fi, err := os.Stat(p)
		if err != nil {
			return false
		}
		f.Files++
		mt := fi.ModTime().UnixNano()
		if f.Newest == 0 || mt > f.Newest {
			f.Newest = mt
		}
		if f.Oldest == 0 || mt < f.Oldest {
			f.Oldest = mt
		}
		if d.SinceEpoch != 0 && float64(mt)/1e9 < d.SinceEpoch {
			return false
		}
		f.InWindow++
		return true
	}
	for _, p := range codex {
		if keep(p, &folders[codexFolder[p]]) {
			out = append(out, Source{"codex", p})
		}
	}
	for _, p := range cl {
		if keep(p, &folders[ci]) {
			out = append(out, Source{"claude", p})
		}
	}
	return out, folders
}

func fold(p string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// walk returns regular files under dir named *.jsonl or *.jsonl.zst, skipping names that start with ".",
// following symbolic links to folders (as glob does) without looping. Paths keep dir as given as their prefix.
func walk(dir string) ([]string, int, error) {
	if dir == "" {
		return nil, 0, os.ErrNotExist
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return nil, 0, err
	}
	if !fi.IsDir() {
		return nil, 0, &fs.PathError{Op: "read", Path: dir, Err: errNotDir}
	}
	if _, err := os.ReadDir(dir); err != nil {
		return nil, 0, err
	}
	unreadable := 0
	var out []string
	visited := map[string]bool{}
	var rec func(abs string)
	rec = func(abs string) {
		real, err := filepath.EvalSymlinks(abs)
		if err != nil || visited[real] {
			return
		}
		visited[real] = true
		ents, err := os.ReadDir(abs)
		if err != nil {
			unreadable++
			return
		}
		for _, e := range ents {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			p := join(abs, name)
			typ := e.Type()
			if typ&fs.ModeSymlink != 0 {
				fi, err := os.Stat(p)
				if err != nil {
					continue
				}
				if fi.IsDir() {
					rec(p)
					continue
				}
				typ = fi.Mode().Type()
			}
			if e.IsDir() {
				rec(p)
			} else if typ.IsRegular() && (strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst")) {
				out = append(out, p)
			}
		}
	}
	rec(dir)
	return out, unreadable, nil
}

var errNotDir = errorString("not a folder")

type errorString string

func (e errorString) Error() string { return string(e) }

// join is os.path.join(a, b) without cleaning a (the reference keeps the folder prefix as written).
func join(a, b string) string {
	if strings.HasSuffix(a, string(os.PathSeparator)) || (runtime.GOOS == "windows" && strings.HasSuffix(a, "/")) {
		return a + b
	}
	return a + string(os.PathSeparator) + b
}
