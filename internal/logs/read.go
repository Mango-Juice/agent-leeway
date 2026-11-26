// Package logs reads Claude Code transcripts and Codex rollouts (read-only) into per-file records:
// one thread, its model responses (steps), tool calls, user turns, commits and the events that can
// precede a cache rewrite. The parsing rules are the ones of the Python reference
// (research/oracle/agent_leeway/pipeline/normalize.py and cache_causes.py); differences are listed in docs/17.
package logs

import (
	"bufio"
	"bytes"
	"errors"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"

	"github.com/klauspost/compress/zstd"
)

// Line is one decoded line of a log file.
type Line struct {
	V   py.Value
	Err bool // not valid JSON (a broken or partial line)
}

// ReadLines streams the lines of a plain or zstd-compressed (*.zst) JSONL file, calling fn for each line
// that is a JSON value. Lines are split on "\n" only, a trailing "\r" is removed, invalid UTF-8 is replaced as
// Python's errors='replace' does, and there is no line-length limit (lines of many MB are normal).
// stats counts what could not be used.
func ReadLines(path string, st *Stats, fn func(v py.Value)) error {
	return scan(path, func(line []byte) { handle(line, st, fn) })
}

// ReadRawLines calls fn with every line as text (decoded like ReadLines, "\r\n" trimmed, not parsed).
func ReadRawLines(path string, fn func(line string)) error {
	return scan(path, func(line []byte) {
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
		fn(py.DecodeBytes(line))
	})
}

// MaxLine bounds one line in memory; longer lines are skipped (and counted as broken) instead of exhausting
// memory, e.g. a small .zst that expands into one endless line.
var MaxLine = 256 << 20

// oversized is passed to each for a line that exceeded MaxLine (it is not JSON, so it counts as broken).
var oversized = []byte("\x00oversized line")

func scan(path string, each func(line []byte)) error {
	f, err := os.Open(path) // read-only; Go opens with read/write share flags on Windows
	if err != nil {
		return err
	}
	defer f.Close()
	_, _, err = scanUpTo(f, path, -1, false, each)
	return err
}

// scanUpTo is scan over the first limit bytes of an open file's (decompressed) content, all of it when limit < 0,
// from where the file is positioned. It returns how many bytes it read and their checksum, so a second read of the
// same open file can stop where the first one did (a session being written grows in between) and tell when the
// file was rewritten in place instead. The checksum is computed only when sum is set (0 otherwise).
func scanUpTo(f *os.File, path string, limit int64, sum bool, each func(line []byte)) (int64, uint32, error) {
	var r io.Reader = f
	if strings.HasSuffix(path, ".zst") {
		d, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(8<<30))
		if err != nil {
			return 0, 0, err
		}
		defer d.Close()
		r = d
	}
	if limit >= 0 {
		r = io.LimitReader(r, limit)
	}
	cr := &countReader{r: r}
	if sum {
		cr.h = crc32.NewIEEE()
	}
	br := bufio.NewReaderSize(cr, 1<<20)
	var long []byte
	skipping := false
	for {
		chunk, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			if !skipping && len(long)+len(chunk) > MaxLine {
				skipping, long = true, nil
			}
			if !skipping {
				long = append(long, chunk...)
			}
			continue
		}
		if skipping {
			each(oversized)
			skipping = false
			if err != nil {
				return cr.n, cr.sum(), nil
			}
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return cr.n, cr.sum(), err
		}
		line := chunk
		if long != nil {
			long = append(long, chunk...)
			line = long
		}
		if len(line) > 0 {
			each(line)
		}
		long = long[:0]
		if long != nil && cap(long) > 64<<20 {
			long = nil // do not keep a huge buffer around
		}
		if err != nil { // EOF
			return cr.n, cr.sum(), nil
		}
	}
}

// countReader counts what is read through it, and checksums it when h is set.
type countReader struct {
	r io.Reader
	n int64
	h hash.Hash32
}

func (c *countReader) Read(p []byte) (int, error) {
	k, err := c.r.Read(p)
	c.n += int64(k)
	if c.h != nil {
		c.h.Write(p[:k])
	}
	return k, err
}

func (c *countReader) sum() uint32 {
	if c.h == nil {
		return 0
	}
	return c.h.Sum32()
}

func handle(line []byte, st *Stats, fn func(v py.Value)) { handleWith(py.Loads, line, st, fn) }

// handleOwned is handle with strings that do not share memory with the line (py.LoadsOwned): what a reader keeps
// from a line does not keep the line alive.
func handleOwned(line []byte, st *Stats, fn func(v py.Value)) {
	handleWith(py.LoadsOwned, line, st, fn)
}

func handleWith(loads func(string) (py.Value, error), line []byte, st *Stats, fn func(v py.Value)) {
	line = bytes.TrimSuffix(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	if len(bytes.TrimSpace(line)) == 0 {
		st.Blank++ // json.loads('') fails in Python too; nothing to report
		return
	}
	s := py.DecodeBytes(line)
	v, err := loads(s)
	if err != nil {
		st.Broken++
		return
	}
	fn(v)
}

// Stats counts lines and records that were not used, so they are reported instead of dropped silently.
type Stats struct {
	Blank     int
	Broken    int            // not valid JSON (often a partial last line of a session being written)
	NotObject int            // valid JSON that is not an object
	Unknown   map[string]int // record types this version does not know
	// UsageOnlyNew counts Codex files whose token usage appears only in token_usage_record (a format this
	// version does not read): their responses are missing from the totals.
	UsageOnlyNew int
}

func (s *Stats) unknown(kind string) {
	if s.Unknown == nil {
		s.Unknown = map[string]int{}
	}
	s.Unknown[kind]++
}

// Add merges o into s.
func (s *Stats) Add(o Stats) {
	s.Blank += o.Blank
	s.Broken += o.Broken
	s.NotObject += o.NotObject
	s.UsageOnlyNew += o.UsageOnlyNew
	for k, v := range o.Unknown {
		s.unknown(k)
		s.Unknown[k] += v - 1
	}
}
