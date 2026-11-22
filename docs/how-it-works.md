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

Habits are reported for the conversations you had yourself. Automated runs (scripts, `claude -p`, `codex exec`, orchestrator workers) are counted separately, and the drift check skips them unless you pass `--include-auto`.

- The split uses the launcher fields in the logs (Claude Code's `entrypoint`, Codex's `originator` and session source) and worker phrases in prompts.
- The providers do not document these fields, so the lists are settings, with the values seen in real logs as defaults ([commands.md](commands.md#settings-file)).
- A session whose launcher is on neither list is counted as a direct conversation and marked uncertain. `leak` prints how many such sessions there were and their spend.

## The drift check

`agent-leeway drift` uses a model as the judge, through your own CLI. Claude Code sessions go to `claude -p`, Codex sessions to `codex exec`. A session never goes to the other provider. Default judges: `claude:opus` and `codex:gpt-6.2-astra`.

- The judge follows [rubric.md](rubric.md) and must quote the transcript for every event. The program checks each quote, drops any event whose quote is not in the transcript, and works out the session's verdict from the events.
- Each moment you had to steer the agent back gets one of three causes. The agent broke an instruction it had, or made its own mistake. The instruction was missing or ambiguous; you get a sentence to add to `CLAUDE.md` or `AGENTS.md`. Or the record is not enough to tell.
- Bugs you report are counted as defect reports, not as drift. A bug in exactly what the agent said it had checked is a false completion, and that counts as drift.
- Sessions in any language can be judged. Quotes stay as written. Summaries and suggested sentences come in the language you mostly wrote in during that session.
- A transcript over 160,000 characters is judged with its middle left out. The summary and report say how many sessions this happened to.
- Before the first call, the program runs the CLI's `--version` and help (and `codex features list`) and checks that every option the lock-down uses exists. If one is missing it stops and names it. Tested with Claude Code 2.1.329 and codex-cli 0.171.0.

## Grouping

After judging, the drift check makes one more call per tool. It groups the judged problems into themes: requests you had to make in two or more sessions, in your own words. The report then says, for example, "you had to say this in 3 sessions".

- The program keeps a theme only if your own words state the request in at least two sessions. A shared kind of agent mistake is not a theme.
- Grouping is on for both tools. Turn it off with `--no-grouping`, for one tool with `--grouping claude=off`, or for good with `"grouping": {"claude": false}` in settings.json.
- `--group-only` groups the sessions already judged and judges nothing.
- Results are kept in `drift/groups.jsonl` in the cache. `report` reads them and calls no model.

## Before a drift run

The drift check shows how many sessions are in the window, how many it will judge, the grouping calls and an estimate, and asks. Without a terminal it refuses unless you pass `--yes`.

- It takes the most recent sessions not judged yet first, 50 by default (`--max-sessions`), so a second run continues with older ones. When `--max-sessions` or `--budget` leaves sessions out, it says so.
- It runs up to 3 calls at once (`--concurrency`).
- It starts no new call when the `--budget` would be exceeded, when a provider reports a usage or rate limit, or after two failures in a row.
- Each call has a 30-minute limit (`--timeout`). On timeout, Ctrl-C or termination, the judge and everything it started are ended.
- Results are saved per session. Running the same command again judges only what is left.

## What a drift run uses

One judge call per session, plus one grouping call per tool, through the `claude` or `codex` CLI you are logged in with.

- On a subscription, this counts against your plan's usage.
- With an API key, it costs roughly $0.15 to $0.35 per session at list price. On the validation runs: Claude Opus $0.24–$0.32, gpt-6.2-astra $0.15–$0.21 per session.
- Judging the same sessions again within the provider's cache lifetime costs much less.
- The estimate shown before a run is an upper estimate for the input.
