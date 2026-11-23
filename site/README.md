# Example report

The example site shows a real Leeway report, rendered by the released binary, of a person who does not exist.
Every session, path, project, quote and judgment in it is made up. Nothing comes from anyone's logs.

## How it is made

```sh
site/build.sh [BINARY] [OUT]     # default: build ./cmd/agent-leeway, write to ./site-out
```

It needs python3 3.7 or later, and Go when no binary is given.

1. `example.py logs` writes made-up Claude Code transcripts and Codex sessions in the formats the tools write.
   The made-up person works on two projects: a web app with Claude Code and a Go service with Codex.
   The logs cover the last 60 days, so the report also has a previous period to compare with.
2. Two visits a month apart, as a person would make them: 30 days before the build date, `agent-leeway drift` checks
   that month and `agent-leeway report` runs; on the build date the same again. The first check's record is what the
   in-depth side compares with. `drift` judges the sessions. The judge is `example.py judge`, plugged in through `LEEWAY_JUDGE_CMD`, the
   tool's own stand-in hook. It answers each session with a judgment written by hand in `example.py`. It never calls
   a model and costs nothing. The answers go through the same checks as a real judge's: every quote must be found
   in the session, and a theme must be stated in the person's own words in two sessions.
3. `agent-leeway report` renders the report in English and Korean. The HTML is copied unchanged.
4. `example.py frame` puts a thin bar around it: "Example · made-up data" (with a note that the sessions and
   judgments are written by hand), language switch, install line, link to the repository.
5. `example.py check` fails the build when the example stops showing what it is meant to show (below).

Everything runs offline, with a clean environment and a temporary home folder, so your own settings, logs and cache
(`~/.claude`, `~/.codex`, `~/.cache`, and variables such as `CLAUDE_CONFIG_DIR` or `CODEX_HOME`) are never read or
written.

## What the example must show

The made-up data is chosen so that one page shows what the tool is for, without looking staged. `check` asserts each of
these:

- both parts checked, Fair on both, Fair overall;
- a first fix built on one request asked for in three sessions in different words (a theme), with a draft rule;
- a rule that was already in CLAUDE.md and was still broken;
- a cost leak with a dollar amount (sessions resumed after long breaks);
- a "going well" section, and exactly one problem that was never fixed;
- evidence dialogs with the person's own words;
- a comparison on both cards: cost "Better" against the previous 30 days; in-depth both rates against the first check
  without a word (the change is within normal swings for 21 sessions on each side); some kinds of problem fewer than
  at the first check, some more;
- the first screen within its text budget (75 English words, 155 Hangul syllables);
- every session judged once with its hand-written verdict, and the same verdicts in English and Korean;
- no real paths and no external assets; every file readable by everyone.

It runs on every build, so a change in the report that hides one of them fails CI instead of
quietly changing the example.

## Files

| File | What it is |
|:--|:--|
| `example.py` | made-up sessions and hand-written judgments (en, ko); log writer; stand-in judge; frame; check |
| `build.sh` | the whole build, offline |
| `frame.html` | the bar around the report (no external assets, no trackers, CSP `default-src 'none'`) |

## Where it is published

`.github/workflows/pages.yml` builds the site and deploys it to GitHub Pages (https://mango-juice.github.io/agent-leeway/)
when a release is published, or by hand for an existing release tag (Actions → example site → Run workflow). It
downloads that release's `agent-leeway_linux_amd64.tar.gz`, checks it against the release's `checksums.txt` and its
build provenance, and builds with it, so the site always shows a released version. Prereleases are skipped. Nothing generated is committed.

Two repository settings are needed once:

1. Settings → Pages: source "GitHub Actions".
2. Settings → Environments → `github-pages`: add a deployment tag rule `v*`. By default the environment accepts only
   the default branch, and a release run comes from its tag.

Without the first, the workflow stops with a message saying so.

The `example-site` job in `ci.yml` runs the same build with the binary from each commit, so a change that breaks the
example fails CI.
