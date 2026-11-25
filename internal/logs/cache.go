package logs

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/Mango-Juice/agent-leeway/internal/meta"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// cacheVersion changes whenever parsing rules change, so old entries are ignored.
const cacheVersion = 14

// FileCache keeps each parsed log file under <dir>/files, keyed by path and checked against the file's
// size and modification time: an unchanged file is not read again. Local only.
type FileCache struct{ dir string }

// NewFileCache uses <cacheDir>/files.
func NewFileCache(cacheDir string) *FileCache {
	d := filepath.Join(cacheDir, "files")
	_ = os.MkdirAll(d, 0o700)
	return &FileCache{dir: d}
}

type entry struct {
	Version int
	Binary  string // the program version that wrote the entry: another build re-reads the file
	Path    string
	Size    int64
	MTime   int64
	File    File
}

func (c *FileCache) name(path string) string {
	h := sha1.Sum([]byte(path))
	return filepath.Join(c.dir, hex.EncodeToString(h[:])+".gob")
}

// Stamp is a file's size and modification time.
type Stamp struct {
	Size, MTime int64
	OK          bool
}

// StampOf reads a file's stamp (taken before parsing, so a line written during the parse is not hidden).
func StampOf(path string) Stamp {
	s, m, ok := stamp(path)
	return Stamp{s, m, ok}
}

func stamp(path string) (int64, int64, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}
	return fi.Size(), fi.ModTime().UnixNano(), true
}

// Get returns the cached parse of path if the file has not changed since.
func (c *FileCache) Get(path string) *File {
	size, mt, ok := stamp(path)
	if !ok {
		return nil
	}
	b, err := os.ReadFile(c.name(path))
	if err != nil {
		return nil
	}
	var e entry
	if gob.NewDecoder(bytes.NewReader(b)).Decode(&e) != nil || e.Version != cacheVersion || e.Binary != meta.Version || e.Path != path || e.Size != size || e.MTime != mt {
		return nil
	}
	return &e.File
}

// Put stores a parse under the stamp the file had before it was read. If the file has changed since, the
// parse may lack the newest lines and is not stored. (Written to a temporary file first, then renamed.)
func (c *FileCache) Put(path string, f *File, before Stamp) {
	size, mt, ok := stamp(path)
	if !ok || !before.OK || size != before.Size || mt != before.MTime {
		return
	}
	var buf bytes.Buffer
	if gob.NewEncoder(&buf).Encode(entry{Version: cacheVersion, Binary: meta.Version, Path: path, Size: size, MTime: mt, File: *f}) != nil {
		return
	}
	tmp := c.name(path) + ".tmp"
	if os.WriteFile(tmp, buf.Bytes(), 0o600) == nil {
		_ = os.Rename(tmp, c.name(path))
	}
}

func init() {
	gob.Register(py.Value{})
}

// Forget removes the cached parse of path.
func (c *FileCache) Forget(path string) { _ = os.Remove(c.name(path)) }

// Prune removes cache entries of files that are no longer listed (deleted sessions, rollouts Codex compressed).
func (c *FileCache) Prune(keep []Source) {
	want := map[string]bool{}
	for _, s := range keep {
		want[filepath.Base(c.name(s.Path))] = true
	}
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !want[e.Name()] {
			_ = os.Remove(filepath.Join(c.dir, e.Name()))
		}
	}
}
