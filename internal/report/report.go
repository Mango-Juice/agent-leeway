// Package report builds the combined report (docs/24): a model of ranked findings with per-tool advice, laid out
// as a Markdown file, an HTML file and a terminal summary. Files are written locally; nothing is uploaded.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Build renders the model as Markdown and HTML (the language is the model's).
func Build(m *Model) (string, string, error) {
	h, err := HTML(m)
	return Markdown(m), h, err
}

// Write saves both files as report-<local time>.md/.html (UTF-8, no BOM), readable only by the user (they
// quote the user's own prompts). An existing name gets a numeric suffix; nothing is overwritten and a link
// planted at the name is not followed.
func Write(mdText, htmlText, dir string, now time.Time) (string, string, error) {
	if dir == "" {
		dir = config.ReportDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	stem := now.Local().Format("report-2006-01-02T1504")
	for i := 1; i < 1000; i++ {
		name := stem
		if i > 1 {
			name = fmt.Sprintf("%s-%d", stem, i)
		}
		pm, ph := filepath.Join(dir, name+".md"), filepath.Join(dir, name+".html")
		fm, err := os.OpenFile(pm, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		fh, err := os.OpenFile(ph, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			fm.Close()
			os.Remove(pm)
			continue
		}
		if err != nil {
			fm.Close()
			return "", "", err
		}
		_, e1 := fm.WriteString(py.ReplaceSurrogates(mdText, "\uFFFD"))
		_, e2 := fh.WriteString(py.ReplaceSurrogates(htmlText, "\uFFFD"))
		e3, e4 := fm.Close(), fh.Close()
		for _, e := range []error{e1, e2, e3, e4} {
			if e != nil {
				return "", "", e
			}
		}
		return pm, ph, nil
	}
	return "", "", fmt.Errorf("too many reports named %s in %s", stem, dir)
}
