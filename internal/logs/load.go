package logs

import (
	"runtime"
	"sync"
)

// Load parses sources in parallel (one file per worker at a time) and returns them in source order.
// A file that cannot be read is skipped and counted. cache may be nil.
func Load(srcs []Source, workers int, cache *FileCache) (files []*File, skipped int) {
	files, bad := LoadReport(srcs, workers, cache)
	return files, len(bad)
}

// LoadReport is Load, naming the files that could not be read.
func LoadReport(srcs []Source, workers int, cache *FileCache) (files []*File, skipped []string) {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	out := make([]*File, len(srcs))
	var wg sync.WaitGroup
	ch := make(chan int)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				out[i] = loadOne(srcs[i], cache)
			}
		}()
	}
	for i := range srcs {
		ch <- i
	}
	close(ch)
	wg.Wait()
	for i, f := range out {
		if f == nil {
			skipped = append(skipped, srcs[i].Path)
			continue
		}
		files = append(files, f)
	}
	return files, skipped
}

func loadOne(s Source, cache *FileCache) *File {
	before := StampOf(s.Path)
	if cache != nil {
		if f := cache.Get(s.Path); f != nil {
			return f
		}
	}
	var f *File
	var err error
	if s.Tool == "codex" {
		f, err = ParseCodex(s.Path)
	} else {
		f, err = ParseClaude(s.Path)
	}
	if err != nil {
		return nil
	}
	if cache != nil {
		cache.Put(s.Path, f, before)
	}
	return f
}
