# Changelog

All notable changes to Leeway (`agent-leeway`) are recorded here. Versions follow [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-11-22

### Added
- First public version: a single executable for macOS, Linux and Windows (x64 and arm64).
- `leak`: mechanical cost leaks from local Claude Code and Codex logs, by cause and habit, priced from a built-in price table with sources (`prices`, `prices --edit`).
- `drift`: judges recent sessions against the bundled rubric v2.1, through each session's own provider CLI.
  - The 50 most recent sessions by default (`--max-sessions`).
  - An optional `--budget`.
  - An estimate and a confirmation before any call.
  - `--write-inputs` to read the exact judge input first.
  - From the skill, only the calling tool's sessions (`--tool claude` or `--tool codex`; `--tool all` for both). From a terminal, both tools.
  - Summaries and suggested sentences in the language the user writes in; field names and quotes unchanged (judged text `v2.1-cf92779d+t2`).
  - A check of the judge CLI before the first call: every lock-down flag must exist. Tested with Claude Code 2.1.329 and codex-cli 0.171.0.
  - Sessions longer than 160,000 characters are judged with the middle omitted; this is recorded and reported.
  - Results and reports say "custom rubric" when the rubric text is not the bundled one.
- Grouping by meaning: after judging, one call per tool groups the judged problems into themes ("you had to say this in N sessions"). On by default for both tools; `--no-grouping`, `--grouping`, `--group-only`, settings.json `"grouping"`.
- `report`: one page with an overall rating, the three changes that would help most with their sources, what is going well, where the agent went off and where the money went, and a look-up section. A self-contained HTML file, a Markdown file and a terminal summary, in English or Korean (`--lang`).
- Keys and tokens in displayed excerpts and summaries are masked (rules based on the gitleaks default set).
- Direct conversations are told from automation by the launcher fields the logs record, with the lists configurable in settings.json `"origin"`; sessions with an unknown launcher are counted as uncertain and reported.
- `leak` and `drift` note when the price table is over 90 days old or a listed promotion has ended.
- `compare`, `idle`, `rubric`, `prices`, `privacy`.
- `install-skills` / `uninstall-skills`: one skill for Claude Code and Codex.
- `self-update --check`. It downloads nothing.
- Install scripts for macOS/Linux (`install.sh`) and Windows (`install.ps1`). Both verify checksums and need no administrator rights.
