# How it works

## The cost check

`agent-leeway leak` is plain code. It calls no model. It reads the local Claude Code and Codex logs, including Codex's compressed `*.jsonl.zst` files and `archived_sessions`, and finds:

- prompt-cache expiry after a long break
- cache rewrites after a mid-session change of model or tool
- repeated commands and consecutive failures
- re-reads of files that did not change
- oversized tool output

Each leak is priced at API list price from a built-in table with sources ([commands.md](commands.md#prices)). The amounts are list-price equivalents, not your bill. Leaks are grouped by the habit that causes them, and the window is compared with the previous window of the same length.

Claude Code deletes transcripts after 30 days by default (`cleanupPeriodDays`). When a window reaches back past the oldest transcript, the output says so.

## Direct conversations and automation

Habits are reported for the conversations you had yourself. Automated runs (scripts, `claude -p`, `codex exec`, orchestrator workers) are counted separately, and the in-depth check skips them unless you pass `--include-auto`.

- The split uses the launcher fields in the logs (Claude Code's `entrypoint`, Codex's `originator` and session source) and worker phrases in prompts.
- The providers do not document these fields, so the lists are settings, with the values seen in real logs as defaults ([commands.md](commands.md#settings-file)).
- A session whose launcher is on neither list is counted as a direct conversation and marked uncertain. `leak` prints how many such sessions there were and their spend.

## The in-depth check (`drift`)

`agent-leeway drift` uses a model as the judge, through your own CLI. Claude Code sessions go to `claude -p`, Codex sessions to `codex exec`. A session never goes to the other provider. The judging model is chosen as described under [Judging models](#judging-models).

- The judge follows [rubric.md](rubric.md) and must quote the transcript for every event. The program checks each quote, drops any event whose quote is not in the transcript, and works out the session's verdict from the events.
- Each moment you had to steer the agent back gets one of three causes. The agent broke an instruction it had, or made its own mistake. The instruction was missing or ambiguous; you get a sentence to add to `CLAUDE.md` or `AGENTS.md`. Or the record is not enough to tell.
- Bugs you report are counted as defect reports, not as drift. A bug in exactly what the agent said it had checked is a false completion, and that counts as drift.
- Sessions in any language can be judged. Quotes stay as written. Summaries and suggested sentences come in the language you mostly wrote in during that session.
- A transcript over 160,000 characters is judged with its middle left out. The summary and report say how many sessions this happened to.
- Before the first call, the program runs the CLI's `--version` and help (and `codex features list`) and checks that every option the lock-down uses exists. If one is missing it stops and names it. Tested with Claude Code 2.1.329 and codex-cli 0.171.0.

## Grouping

After judging, the in-depth check makes one more call per tool. It groups the judged problems into themes: requests you had to make in two or more sessions, in your own words. The report then says, for example, "you had to say this in 3 sessions".

- The program keeps a theme only if your own words state the request in at least two sessions. A shared kind of agent mistake is not a theme.
- Grouping is on for both tools. Turn it off with `--no-grouping`, for one tool with `--grouping claude=off`, or for good with `"grouping": {"claude": false}` in settings.json.
- `--group-only` groups the sessions already judged and judges nothing.
- Results are kept in `drift/groups.jsonl` in the cache. `report` reads them and calls no model.

## Before an in-depth check

The in-depth check shows how many sessions are in the window, how many it will judge, the grouping calls and an estimate, and asks. Without a terminal it refuses unless you pass `--yes`.

- It takes the most recent sessions not judged yet first, 50 by default (`--max-sessions`), so a second run continues with older ones. When `--max-sessions` or `--budget` leaves sessions out, it says so.
- It runs up to 3 calls at once (`--concurrency`).
- It starts no new call when the `--budget` would be exceeded, when a provider reports a usage or rate limit, or after two failures in a row.
- Each call has a 30-minute limit (`--timeout`). On timeout, Ctrl-C or termination, the judge and everything it started are ended.
- Results are saved per session. Running the same command again judges only what is left.

## What an in-depth check uses

One judge call per session, plus one grouping call per tool, through the `claude` or `codex` CLI you are logged in with.

- On a subscription, this counts against your plan's usage.
- With an API key, it costs roughly $0.15 to $0.35 per session at list price. On the validation runs: Claude Opus $0.24–$0.32, gpt-6.2-astra $0.15–$0.21 per session.
- Judging the same sessions again within the provider's cache lifetime costs much less.
- The estimate shown before a run is an upper estimate for the input.

## Judging models

The rubric was validated with two models: Claude Opus 5.5 (`claude-opus-5-5`) for Claude Code sessions and GPT-6.2 Astra (`gpt-6.2-astra`, reasoning medium) for Codex sessions ([validation.md](validation.md)). Other models can judge, but their results have not been checked with our test set.

- **The choice, per tool:** the newest checked version (always tried first: a model missing from the account's list is only a hint); else a newer version of the same family (for Codex from the account's model list, excluding variants such as `-mini`; for Claude Code the `opus` alias); else the CLI's own default model. `--model` or a `judge` entry in settings.json overrides the choice.
- **No paying twice:** a session already checked under the same rubric by any model of its tool counts as checked, so a change of model does not check it again. `drift --recheck` checks such sessions again with the current model. The report lists every model that judged.
- **Before you agree:** the dry run prints the expected judging model ("Expected judging model for Codex sessions: GPT-6.2 Astra"). For Codex it reads the account's model list with `codex debug models`, which makes no model call; for Claude Code there is no such list, so the first call tells.
- **No silent switch:** if the CLI refuses the model during the run (unknown to that CLI version, or not allowed for the account), the run stops, nothing more is judged, and you are asked before another model judges (in a terminal, a question; with `--yes`, exit code 4 and the command to run). The refusal is remembered per CLI version for 7 days, so the next dry run already names the next model; `--model` ignores it.
- **Older Codex CLIs:** before 0.171.0, Codex has no catalog entry for GPT-6.2 Astra. It still uses that model, with default settings, and says "Model metadata for `gpt-6.2-astra` not found". The run continues and shows that notice once; updating Codex (`codex update`) gives the setup that was validated.
- **The report** names the model that judged ("checked by GPT-6.2 Astra"); a version not yet tested with our test set gets "(this version not yet tested)".
- **Prices:** a model without a row in the price table gets no dollar estimate, only the plan-usage note.

## Compared with the previous period

The report compares this period with the period just before it, of the same length (with `--since 30d`, the 30 days before). Lower is better on both sides. Windows shorter than a day are not compared. A session with turns on both sides of the boundary counts once, in the period of its last turn.

- **On the first screen:** each card gets one line, for example "✓ Better · 33% → 21%", and the dates say "compared with the 30 days before". States are Better ✓, Worse ! and Similar =; the icon and the word carry the state, the colour only repeats it. The state is decided on the rates as shown.
- **In the whole picture:** under each block, "Compared with the previous 30 days" lists the headline rate and up to three rows that changed most (problem kinds as a share of checked sessions; habits as cost per $100 spent, so less work does not look like less waste).
- **Better or worse, not noise:** a change counts only when it passes the threshold below and is larger than what one session could move on its own: one checked session of the smaller period on the in-depth side, the largest single session's waste of the smaller period on the cost side. Otherwise it is Similar.
- **Cost side:** from the logs, so it works on the first run. Similar when the waste share moves less than 2 points; habits move less than $1.00 per $100, and those under $0.50 per $100 in both periods are left out. No comparison when the previous period spent less than $5 or had fewer than 3 sessions.
- **Deleted Claude Code logs:** Claude Code deletes transcripts after `cleanupPeriodDays` (30 by default). The logs count as covering a period when the oldest transcript reaches back to it, or when that setting reaches back past it and its `settings.json` was last changed before the period began (a raised setting does not bring back deleted logs). Otherwise the cost side uses a numbers-only snapshot that an earlier report saved for a window of the same length from the same log folders, starting at most 10% of the window (and 2 days) apart, or it covers Codex only and says so; the in-depth side leaves Claude Code out and says so. A report saves a snapshot only for a window its logs fully cover.
- **In-depth side:** from the in-depth check results already stored for sessions in the previous period, only for tools checked in both periods with the same judging model (the exact model when the CLI recorded it) and rubric, with at least 8 checked sessions and at least 60% of the sessions checked in each period. Similar when the serious-miss rate moves less than 8 points.
- **When there is no sound comparison:** the first screen shows nothing for that card, and the whole picture says why in one line.
