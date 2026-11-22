package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
	"github.com/Mango-Juice/agent-leeway/internal/config"
)

func init() {
	register(command{name: "prices", help: "show the price table (USD per million tokens, with sources), or copy it for editing", run: cmdPrices})
}

func cmdPrices(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("prices", flag.ContinueOnError)
	fs.SetOutput(errw)
	copyTo := fs.Bool("edit", false, "copy the built-in table to "+config.PricesPath()+" (an existing file is kept)")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	if *copyTo {
		p := config.PricesPath()
		if _, err := os.Stat(p); err == nil {
			fmt.Fprintf(errw, "%s already exists; edit it directly.\n", p)
			return 1
		}
		if err := os.MkdirAll(config.ConfigDir(), 0o700); err == nil {
			err = os.WriteFile(p, assets.Prices, 0o644)
			if err == nil {
				fmt.Fprintf(out, "Wrote %s. Change a row (and its \"read\" date); agent-leeway uses this file instead of the built-in table.\n", p)
				return 0
			}
		}
		fmt.Fprintf(errw, "could not write %s\n", p)
		return 1
	}
	t := config.Prices()
	src := "built in"
	if t.Path != "" {
		src = t.Path
	}
	if err := config.PricesError(); err != nil {
		fmt.Fprintf(errw, "Warning: %s could not be read (%v); the built-in table is used.\n", config.PricesPath(), err)
	}
	fmt.Fprintf(out, "Price table (%s), read %s. USD per million tokens: input / cache read / cache write / output.\n", src, t.Read)
	fmt.Fprintf(out, "Unknown models are priced like: Claude Code %s, Codex %s (and reported).\n\n", t.Fallback["claude"], t.Fallback["codex"])
	for _, m := range t.Models {
		w := fmt.Sprintf("%g", m.CacheWrite)
		if m.Provider == "anthropic" {
			w = fmt.Sprintf("%g (5m) %g (1h)", m.CacheWrite5m, m.CacheWrite1h)
		} else if m.CacheWrite == 0 {
			w = "none"
		}
		line := fmt.Sprintf("  %-18s %g / %g / %s / %g", m.ID, m.Input, m.CacheRead, w, m.Output)
		if m.Long != nil {
			line += fmt.Sprintf("; above %dk tokens %g / %g / %g", m.Long.Above/1000, m.Long.Input, m.Long.CacheRead, m.Long.Output)
		}
		if m.TTLMinutes > 0 {
			line += fmt.Sprintf("; cache %g min", m.TTLMinutes)
		}
		if m.PromoUntil != "" {
			line += "; promotional until " + m.PromoUntil
		}
		fmt.Fprintln(out, line)
	}
	fmt.Fprintln(out, "\nSources:")
	seen := map[string]bool{}
	for _, m := range t.Models {
		if !seen[m.Source] {
			seen[m.Source] = true
			fmt.Fprintln(out, "  "+m.Source)
		}
	}
	for _, k := range []string{"claude_source", "openai_source"} {
		fmt.Fprintln(out, "  "+t.Caching[k])
	}
	return 0
}
