# Changelog

All notable changes to Leeway (`agent-leeway`) are recorded here. Versions follow [Semantic Versioning](https://semver.org/).

## [0.1.14] - 2026-11-26

### Changed
- The first run (with nothing cached yet) uses far less memory with large Codex logs: about 1.1 GB instead of about 8.4 GB in a test with 1,555 log files in the report's period, because Codex log files are now read line by line instead of being held in memory whole. In exchange it uses more processor time, since each line is read twice, so the first run can take up to about 15% longer. Results are unchanged.

### Fixed
- Codex: a sub-agent's log copies its parent's records, and those copies were also kept in the list of events used to explain cache resets (new user turns, developer messages, settings and model changes), because the log's own thread id was read from its file name one character short. They are now left out, as they already were everywhere else. In the logs we tested, no number in the reports changed: the cause shown for a cache reset could only differ when a copied record is timed after the sub-agent's first own response, and the amounts stay the same. The first run after the update reads the logs again.

## [0.1.13] - 2026-11-26

### Fixed
- Unused skills (Codex): a skill used through a relative path in a call that runs in its skills folder, home or `$CODEX_HOME` (`cd …/skills && python foo/run.py`, `../foo/run.py` from a sibling skill, `-C` after `env`, `make`, `git` or `tar`, or the call's working folder: `workdir` or `cwd`, also in a code-mode script, and the command's recorded `cwd`), a folder named only that way with characters a plain-text key cannot hold, a custom-root skill named by a `file:///` URL, a custom-root skill named late in a very long session, and a skill shown only by a `world_state` entry under a custom root were reported as not used. A `file:` URL's percent escapes are all decoded.
- A test that changed the clock's time zone while its local web server could still read it failed under `go test -race`; each case now runs on its own.

## [0.1.12] - 2026-11-26

### Added
- "Your setup" shows the Claude Code skills not used in the period, under the skill list, even when the list is within its budget: how many of the listed skills, their share of the list's characters, and the unused ones by plugin ("none of this plugin's 3 skills was used in the last 30 days"), with the official way to turn them off. Uses count from every log file of the period (automations and sub-agent threads included): a Skill call, a command you typed or a skill you picked, and for Codex a tool call naming the skill's folder. Shown when at least 20 sessions in the period recorded the list. When the list is over its budget, the fix says the same.
- The same "Unused skills" row for Codex, from the list Codex sent and the skills picked or whose folder a tool call named, with Codex's own way to turn them off (`[[skills.config]]`, `enabled = false`).
- When half or more of a tool's listed skills went unused, a card at the end of Other things worth fixing and one line in the terminal summary; never among the first three fixes or in the rating.

### Changed
- A skill started by typing its command now counts as a use, as well as the agent's skill call, so "used least first" and "not used" count both.
- "Used least first" (the skills whose descriptions were dropped) now counts uses in every log file of the period, automations and sub-agent threads included.

## [0.1.11] - 2026-11-25

### Added
- The two cards on the report's first screen show the rating cut-offs on their scale (10% and 25% of sessions; 5% and 15% of spend), so you can see why a number is Good, Fair or Poor.

### Fixed
- Look things up said money lost to habits is Good "under 5% of spend"; the rating counts exactly 5% as Good, and the table now says "5% of spend or less".

## [0.1.10] - 2026-11-24

### Added
- The example site has a favicon and a link-preview image (English and Korean), so a shared link shows a card.

## [0.1.8] - 2026-11-23

### Added
- "Your setup" in the basic report: each tool's skill list and instruction file against the line its own docs publish (Claude Code: skill descriptions dropped from the list, `CLAUDE.md` over 200 lines; Codex: `AGENTS.md` at `project_doc_max_bytes`). An item over its line is a fix, ranked after the firm and measured fixes; every item is a row in Look things up, with the tool's page. Offline, no model call; see docs/privacy.md for what is read.
- A new cause in "money wasted by habits": screenshots (and other images) left in the conversation, shown as an estimate ("about $X"). Each image is billed again with every later request until compaction; only those re-reads are counted (never taking the screenshot or its first read), from the image's size in its header and each vendor's published token rule, bounded by the logged usage.

### Changed
- Cost records now name the set of causes they counted, and are saved under a new file name that 0.1.7 does not read. A cost saved by 0.1.7 or earlier is not compared with this version's numbers ("this version counts one more cause").
- `leak --json` gained `cause_set` (2 in this version); `compare` says when two files count different causes and then compares each cause on its own.
- The Windows installer downloads faster in Windows PowerShell 5.1: it no longer draws a progress bar during the download, which slowed it several times over. Your session's own progress setting is left as it was.

## [0.1.7] - 2026-11-23

### Changed
- "Compared with" is rebuilt on records. The in-depth card compares with your last in-depth check made the same way (selection, scope, window length, judging model family, rubric; at most 60 days old; no shared session), labelled with its date ("vs 1 Sep check"); a heavy user whose checks cover the 50 most recent sessions now gets a comparison too. The cost card compares with the previous period from the logs ("vs prior 30 days"), or, when Claude Code has deleted them, with an earlier report ("vs 1 Sep report").
- Each card line always shows both rates and its baseline; the word Better or Worse appears only when the change is bigger than normal swings for that many sessions (twice the standard error and a Fisher exact test, p < 0.05). The in-depth card line needs 15 checked sessions on each side. "Similar" is gone. The detail lists problem kinds as counts, with each side's number of sessions and the baseline's dates.
- A first visit says "Next report compares with this one." instead of blaming deleted Claude Code logs.

### Added
- After each in-depth check, a numbers-only record per tool in `compare/checks.jsonl` (see docs/privacy.md). Results of checks made with 0.1.5 or 0.1.6 are turned into records when it is safe to assume they used the defaults, so a later report can compare with them.

## [0.1.6] - 2026-11-23

### Added
- `self-update --check` prints the one command that updates this copy, chosen from where the program is installed: Homebrew (`brew update && brew upgrade agent-leeway && agent-leeway install-skills`), Scoop (`scoop update; scoop update agent-leeway; …`, or `-g` in an administrator terminal for an install for all users), the one-line installer (on Windows in a form that also runs from bash and cmd), or the Releases page. `--json` prints `{current, latest, newer, method, command, url}`, or `{"error": …}` when the check fails; `--lang ko` speaks Korean.
- Asked to update, the skill runs that command and reports the new version. Nothing checks for updates unless you ask.

### Fixed
- `self-update --check` trusts only a well-formed release tag and this repository's release page; anything else is a failed check, and GitHub's rate limit is named as such.
- `leak` no longer compares with the previous window when Claude Code has deleted part of its logs, the same rule the report uses, and says why. `leak --tool codex` keeps its comparison.

## [0.1.5] - 2026-11-23

### Added
- The report compares this period with the previous period of the same length: one line per card on the first screen ("✓ Better · 33% → 21%") and, in the whole picture, the rates that changed most. It is shown only when the comparison is sound (enough spend and sessions; for the in-depth side, the same judging model and rubric and enough checked sessions in both periods), and otherwise the whole picture says why.
- An example report site, built from made-up sessions with the released binary and published on GitHub Pages at each release: https://mango-juice.github.io/agent-leeway/ (Korean: https://mango-juice.github.io/agent-leeway/ko/). `site/` holds how it is made; CI builds and checks it on every change.

### Fixed
- English report text no longer says "in 1 sessions" or "1 event(s)": counts of one read in the singular, others in the plural.

## [0.1.4] - 2026-11-23

### Fixed
- A Codex "error" item (a non-fatal notice such as codex-cli 0.169.0's "Model metadata for `gpt-6.2-astra` not found") no longer stops the in-depth check as if it were a tool call. The notice is shown once (masked, capped); the session is judged when the turn completes and fails normally when the turn fails. Real tool calls still stop the run.
- Judge failure lines name the underlying error text for both CLIs, and two failed calls in a row name the last error.

### Added
- `drift --debug-log` (or `LEEWAY_DEBUG=1`) writes a diagnostics file without session text; a failed run points to it. The bug-report form asks for it.
- The judging model is chosen per tool from `judges.json` (checked versions first, then a newer version of the same family, then the CLI's default). The dry run names the expected judging model; a model refused during a run stops it and asks before another model judges (exit code 4 with `--yes`). The report names the model and marks versions not yet tested. A session checked by any model under the same rubric is not checked again (`--recheck` does).
- A one-line note when a judge CLI is older than the tested version; for Codex below 0.171.0 it says what updating gives.

### Changed
- The summary and report now use two names: the basic report (cost only) and the in-depth check (whether the agent did what you asked). `report` opens with what the basic report covers and ends by asking whether to run the in-depth check.

## [0.1.3] - 2026-11-23

### Changed
- When some sessions have not been checked for whether the agent did what you asked, the report's summary starts with the check-up's two parts and their state ("1) Checking money lost to habits: done. 2) Checking whether the agent did what you asked: not checked yet (176 Codex sessions)"), the report's card says so in plain words, and `report --json` gives `parts`. The skill leads with that state, then the cost result, what the second part finds and what it takes, then asks.
- `--max-sessions` counts sessions not checked yet, so a second run continues with older sessions instead of finding the most recent ones already done.
- The dry run ends its session list with a count by project for all sessions it would check.

### Fixed
- In the terminal and Markdown summaries, a finding from both tools cites both tools' sources instead of only the Claude Code docs.

## [0.1.2] - 2026-11-23

### Fixed
- A fix built on a repeated request now gives the size of its problem first and the repeated request second ("In 12 of 50 sessions it ignored what you had already said. Of those, 2 had the same request: ..."), so fixes ranked by problem size no longer look out of order when a smaller problem has a larger repeated request.
- Asking an agent for a report now leads to an offer to run the in-depth check. `report` ends its summary with the sessions in the period that were never judged and the dry-run command to see them, and `report --json` gives the counts per tool. The skill runs the report for its own tool, shows the dry run, and asks before judging. With `--tool`, the report leaves out the other tool's drift results and says how to see them.
- The short source label and the one-line reason under each piece of advice follow the finding's tool: a Codex finding cites the Codex or OpenAI docs (or our own measurement, labelled so) instead of the Claude Code docs; a cost finding from both tools says "Official docs" and lists both in its panel.
- The drift dry run ends with how to judge fewer or other sessions (`--max-sessions`, `--since`, `--project`), and the skill offers a smaller run when it asks for approval.

## [0.1.1] - 2026-11-23

### Added
- Homebrew and Scoop installs from this repository, which is also the tap (`Formula/`) and the bucket (`bucket/`):
  - `brew tap mango-juice/agent-leeway https://github.com/Mango-Juice/agent-leeway`, then `brew install mango-juice/agent-leeway/agent-leeway`.
  - `scoop bucket add agent-leeway https://github.com/Mango-Juice/agent-leeway`, then `scoop install agent-leeway`.
  - Both install the released binaries, checked against the release's SHA-256 sums.
  - Publishing a release updates both files from that release's `checksums.txt` (a workflow commit by github-actions[bot], no personal token).
- The one-line installers (`install.sh`, `install.ps1`) finish by adding the skill for Claude Code and Codex when their folders exist (`agent-leeway install-skills --auto`), print what they wrote and how to undo it, and end with the next step: ask your agent for a "leeway report". `LEEWAY_NO_SKILLS=1` skips the skill. A failure of this step leaves the program installed. Homebrew and Scoop print a note to run `agent-leeway install-skills` once.
- `npx skills add Mango-Juice/agent-leeway` finds the skill: the repository copy moved from `skill/` to `skills/agent-leeway/`, where the skills CLI looks.

### Changed
- `install-skills` prints one line per agent with the file it wrote and the command that undoes it. When a skill folder is a link to another copy of this skill (as `npx skills add -g` makes for Claude Code), the link is replaced by a folder of its own, so each agent keeps its own copy. A link to any other skill is left alone.
- When agent-leeway is not installed, the skill tells the agent to show the install line and ask before running it.
- The READMEs are shorter. Install details, commands and settings, how the checks work, and privacy moved to `docs/install.md`, `docs/commands.md`, `docs/how-it-works.md` and `docs/privacy.md`. The validation summary moved to `docs/validation.md`.

### Fixed
- With no drift results in the period, the report and the terminal summary no longer say "0 of 0 sessions". They say that the in-depth check has not been run, give the command to run it, and say that the rating rests on cost only.
- `drift` and `report` follow `--lang` (and the detected language) for every interface line, including the pre-run text, progress, folder notes and the privacy line.

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
