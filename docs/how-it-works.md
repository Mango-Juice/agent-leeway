# How it works

## The cost check

`agent-leeway leak` is plain code. It calls no model. It reads the local Claude Code and Codex logs, including Codex's compressed `*.jsonl.zst` files and `archived_sessions`, and finds:

- prompt-cache expiry after a long break
- cache rewrites after a mid-session change of model or tool
- repeated commands and consecutive failures
- re-reads of files that did not change
- oversized tool output
- screenshots and other images left in the conversation (an estimate, shown as "about $X"): an image stays in the context, and is billed again with every later request, until a compaction. Its size comes from the image header, and its tokens from the vendor's published rule (Anthropic: 28-pixel patches; OpenAI: 32-pixel patches, each model's size limits and multiplier). The estimate is bounded by the logs: an image counts for no more than what its first request added, a later request carries no more image tokens than it read from the cache, and an image stops counting when the input falls by about its size. Only the re-reads are counted, each at that request's own cache-read price, for up to 200 later requests; taking the screenshot and its first read are not. The amount is shown only for models whose image token rule is known; images read by other models are left out.

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

## Your setup

The report checks what loads into every session against the line each tool's own docs publish, with no model call:

- **Claude Code skill list.** Claude Code gives the list about 1% of the context window and drops descriptions past it; the report counts the skills listed without one, minus those you set to `"name-only"` in `skillOverrides`. The Codex skill list is not checked: in the logs, a skill Codex left out cannot be told from one you turned off.
- **Instruction file.** Claude Code's docs say to keep each `CLAUDE.md` under 200 lines; the count is of the lines Claude Code loaded (it strips block comments first), so it can be lower than in an editor. Your user-level `CLAUDE.md` is counted once. Codex stops reading `AGENTS.md` files once their combined size reaches `project_doc_max_bytes` (32 KiB unless set at the top of `config.toml`); the report says "At the limit" when the text Codex sent was cut there. When that text is longer than the limit allows, a different limit was in force (a profile or a `-c` option) and the row is left out.

- **Unused Claude Code skills.** Claude Code's docs say every skill in the list adds to the context on every turn, used or not. Under the skill list, a row counts the skills in it that no session in the period used: neither the agent's skill call nor a command you typed (built-in commands such as `/model` never match a skill; a use logged without the plugin prefix counts for every listed skill with that name after its prefix). Uses count from every log file of the period, whatever the report covers: automations (even without `--include-auto`), sub-agent threads and other projects. So a skill is not called unused when a Skill call, a command you typed, a skill you picked or (Codex) a tool call naming its folder in the logs shows it was used; a path a call gives relative to another folder it names (`cd …/skills && python foo/run.py`) is not matched. The period it names runs back no further than the oldest skill list in the logs. It gives their share of the list's characters, not dollars (characters per token vary too much between languages), and groups them by plugin (the `plugin:` prefix of the name), saying when none of a plugin's skills was used. The row needs at least 20 sessions in the period that recorded the list; a skill listed in fewer than 20 of them (added lately, or one project's own) is not counted either way. When the list is over its budget, the fix says the same. The advice points to `/skills` for your own skills and `/plugin` for a plugin's.
- **Unused Codex skills.** The same row for Codex, from the skill list Codex sent (the developer message, `world_state` and, in its delta form, the entries shown) and two kinds of use: a skill you picked (`$name`, `/skills`), and any tool call whose input names the skill's folder: `skills/<folder>`, the folder holding a `SKILL.md` or `scripts`, or the folder's whole path (a custom root, a read of its `references/`), however the command is written, matched against every folder the skill had in the period. The input is decoded as it was written and normalized once (escapes, backslashes, doubled slashes, home notations, case) before matching. It is not read as a shell command, so a search or an edit that names a skill's folder counts as a use too: when this is wrong, it is usually wrong toward "used". A search of the skills folder that names no skill's folder does not count, and the list text Codex sent is never a use. Skills that came with a plugin are grouped by plugin. A skill from an executor or cloud package is left out: no command touches a folder of its, so a use could not be seen. A session whose list was hidden by config, had every description dropped, or was replaced by the "omitted" note is not counted, and the row says how many. Sub-agent threads count for use, not as sessions. There is still no Codex skill-list row against a budget. The advice is Codex's own: `[[skills.config]]` with `enabled = false` in `config.toml`.
- **Card.** When half or more of a tool's listed skills went unused, the tool also gets a card at the end of Other things worth fixing (our rule of thumb, stated next to the rating cut-offs). It never enters the first three fixes or the rating, and the terminal adds one line for it. A Claude Code skill list that is already a fix gets no second card.

Per project, it reads the latest session in the period, and each row shows the project furthest over. Only the sessions the report covers count: direct conversations (automations with `--include-auto`), and `--tool` and `--project` as given. An item over its line becomes a fix, ranked after every firm and measured fix, at most one in the top three. Every item is a row of "Your setup" in Look things up, with a link to the tool's page. A row is left out when its log record is missing or shaped differently. Not compared with before.

## Compared with the previous period

Each card on the first screen compares with its own baseline and says which, in a few words. Lower is better on both sides. Windows shorter than a day are not compared.

- **What the line shows:** both rates and the baseline, for example "33% → 21% · vs 1 Sep check". The word (Better ✓ or Worse !) is added only when the change is bigger than normal swings for this many sessions. Without the word the line is neutral: no icon, no colour.
- **Cost side:**
  - The baseline is the previous period of the same length, from the logs ("vs prior 30 days").
  - When Claude Code has deleted those logs, the baseline is the most recent earlier report of the same length (at most 60 days old, its period ending before this one starts), labelled with its date ("vs 1 Sep report").
  - A change counts when the waste share moves at least 2 points and more than the single largest session's waste of the smaller period. The previous period needs $5 of spend (its waste share is the baseline, so it must not rest on a few cents), and each period 10 sessions.
- **In-depth side:**
  - The baseline is the most recent earlier in-depth check with the same selection (the 50 most recent not yet checked, by default), the same scope (all projects or one `--project`, with or without `--include-auto`; the report's own `--project` and `--include-auto` must match the check that made its results), the same window length, the same judging model family and the same rubric. It is at most 60 days old and shares no session with this period's checked sessions, and it is labelled with its date ("vs 1 Sep check").
  - Only one judge counts on each side: one result per session, from the judging model and rubric of the check that made it. When this period's sessions were checked with more than one model family, nothing is compared.
  - The card line needs 15 checked sessions on each side, the detail list 10. A change counts when the difference in the serious-miss rate is at least twice its standard error for these numbers of sessions and a two-sided Fisher exact test on the two counts gives p < 0.05.
  - When the judging model is a newer version of the same model, only the detail list is shown, with "The judging model was updated."; an older or unknown version gives the detail list with "Checked with a different version of the judging model." Another model family or rubric is not compared.
- **In the whole picture:** under each block, the baseline with its dates and each side's number of sessions, then the rows that changed most: problem kinds as counts out of each side's checked sessions ("A mistake you had to point out: 4 of 21 → 1 of 24"), sorted by the change in rate, habits as cost per $100 spent. For a partial check one line says how the sessions were chosen ("Each check: the 50 most recent of about 150"). One sentence reminds that changes can reflect different work and small ones can be chance.
- **When nothing is compared:** on a first visit, "Next report compares with this one.", only when this visit's numbers are saved. Otherwise one short line with the cause (another scope or window, an older check than 60 days, too few sessions, another model or rubric). With two tools whose last checks were made separately, one is compared and one line says why the other is not.
- **Deleted Claude Code logs:** Claude Code deletes transcripts after `cleanupPeriodDays` (30 by default). The logs count as covering a period when the oldest transcript reaches back to it, or when that setting reaches back past it and its `settings.json` was last changed before the period began.
- **Another set of causes:** each saved cost record names the set of causes it counted. A report record from 0.1.7 or earlier, which did not count screenshots, is not compared with this version's numbers; the card says "this version counts one more cause" instead. Comparisons from the logs count the same causes on both sides.
- **What is saved for this:** after each in-depth check, one numbers-only record per tool; after each report whose logs cover its period, the cost in numbers (see [privacy.md](privacy.md)). The results cache stays the source of every verdict; the records are used only to compare.
- **Upgrading from 0.1.5 or 0.1.6:** their checks left results but no record. A report rebuilds a record from the cached results of an earlier check only when nothing has to be guessed: the logs of that check's 30-day window are still here, and its sessions are exactly the 50 most recent of that window (or all, when fewer), none checked before, with one judging model and rubric. Otherwise there is no baseline, and the next check starts one.
