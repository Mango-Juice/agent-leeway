// Package messages holds all user-facing text (English). Templates use Python format syntax
// ({name}, {usd:,.2f}, ...) and are filled with py.Format. A translation replaces the values only.
// Generated once from the Python reference (research/oracle/gen_messages.py); edit here from now on.
package messages

// Automation is AUTOMATION.
var Automation = map[string]string{
	"auto:heartbeat":     "Codex automation (heartbeat turns injected into an existing thread)",
	"auto:goal":          "Codex goal auto-continuation",
	"auto:orca_worker":   "worker sessions started by an orchestrator",
	"auto:orca_codex":    "Codex sessions started by an orchestrator",
	"auto:codex_exec":    "non-interactive codex exec runs",
	"auto:agent_opened":  "sessions opened by an agent",
	"auto:worker_marker": "worker sessions started by an orchestrator (worker marker in the prompt)",
	"auto:headless":      "headless claude -p runs",
	"auto:originator":    "Codex sessions started by another program",
}

// AutoTips is AUTO_TIPS.
var AutoTips = map[string]string{
	"expiry":      "Each run continues the same thread after a long gap, so the expired context is cached again ({times}, ${usd:,.2f}). Run each time in a new thread, or pass the needed context as a short summary file.",
	"loops":       "Within a run, the same command repeated or failed again {times} (${usd:,.2f}). Poll less often or check once, and tell the automation to give up until the next run after a failure.",
	"reset_known": "Model, reasoning effort or tools changed during runs ({times}, ${usd:,.2f}). Pin model and effort in the automation settings.",
	"other":       "Re-reads and long outputs ${usd:,.2f}: fix which files to read and how to summarize output in the automation prompt.",
}

// Causes is CAUSES (label, text).
var Causes = map[string][2]string{
	"expiry":        {"Cache expired (resumed after a long break)", "Resumed {n} times after a gap longer than the prompt-cache lifetime (Claude Code: 5 minutes or 1 hour, as each cache write records; Codex: 30 minutes on GPT-5.6 and later). After a long break, leave a short summary and start a new session instead."},
	"reset_known":   {"Cache rewritten mid-session (after a model/settings/tool change)", "The cache was rewritten right after {detail} ({n} times). Choose model, reasoning effort and tools when the session starts and do not change them midway."},
	"reset_unknown": {"Cache rewritten mid-session (cause unknown)", "{n} rewrites with no preceding event in the log; most likely server-side cache eviction. Nothing to fix (amount recorded only)."},
	"repeat":        {"Same command repeated (3rd run onward)", "{n} calls ran a command for at least the third time. If it is status polling, have the agent wait for a completion signal; add \"stop and explain before running the same command a third time\" to your instructions."},
	"fail":          {"Consecutive failures", "{n} calls failed right after another failure. \"Stop and report the cause after two failures in a row\" in your instructions breaks the loop."},
	"reread":        {"Unchanged file read again", "{n} calls re-read the same range of a file that had not changed. Put the key file locations and layout in the first request."},
	"oversize":      {"Oversized tool output (over 10k tokens)", "{n} tool outputs over 10k tokens entered the context and were re-read by every later response. Have logs and test output tailed or summarized."},
}

// CauseClass is CAUSE_CLASS.
var CauseClass = map[string]string{
	"agent_side":     "agent-side",
	"prompt_side":    "prompt-side",
	"undeterminable": "undeterminable",
}

// CauseDetail is CAUSE_DETAIL.
var CauseDetail = map[string]string{
	"instruction_violation":            "instruction violated",
	"missing_or_ambiguous_instruction": "instruction missing or ambiguous",
	"agent_error":                      "agent error",
	"undeterminable":                   "undeterminable",
}

// Cli is CLI.
var Cli = map[string]string{
	"description":      "Occasional check-up of your AI coding agent logs (Claude Code, Codex): mechanical cost leaks and intent drift.",
	"since_format":     "--since format: 30d, 2w, 12h",
	"rubric_exists":    "{path} already exists; nothing was changed. Use another name, or add --force to replace it (the old file is kept as a backup).",
	"rubric_backed_up": "Kept the previous file as {path}.",
	"rubric_copied":    "Copied the rubric to {path}. Edit it, then pass --rubric {path} or set \"rubric\" in the settings file.",
	"rubric_file":      "Rubric file: {path} ({ver})",

	// self-update --check
	"upd_no_replace":    "{name} does not replace itself. Run `{name} self-update --check`: it prints the one command that updates this copy.",
	"upd_failed":        "Could not check for a newer version: {err}",
	"upd_current":       "{name} {cur} is up to date.",
	"upd_newer":         "A newer version is available: {vs} (you have {cur}). Release notes: {url}",
	"upd_newer_short":   "A newer version is available: {vs} (you have {cur}).",
	"upd_run_installer": "To update, run the installer again (it also refreshes the skills):",
	"upd_run_pkg":       "To update, run (it also refreshes the skills):",
	"upd_lag_homebrew":  "The Homebrew formula is updated a few minutes after a release is published. If Homebrew still says agent-leeway is up to date, try again a little later.",
	"upd_lag_scoop":     "The Scoop manifest is updated a few minutes after a release is published. If Scoop still says agent-leeway is up to date, try again a little later.",
	"upd_run_admin":     "To update, run this in a terminal opened as administrator, because it is installed for all users (it also refreshes the skills):",
	"upd_rate_limit":    "GitHub limits how often it answers without a login, and the limit is used up for now; try again within an hour",
	"upd_manual":        "This copy was not installed with Homebrew, Scoop or the one-line installer, so download the new version from the Releases page:",

	"privacy_details": "Details:\n" +
		"  - Judge calls run in a fresh private folder under the cache folder, with the session on stdin only.\n" +
		"  - Grouping calls (one per provider after judging) use the same CLI command line and send only that provider's judged problems: ids, rule and cause codes, the quoted instruction and your quoted words exactly as the judge saw them (not masked), and the judge's summaries. Results are stored in drift/groups.jsonl; `report` reads them and makes no call.\n" +
		"  - Enforced by the Claude CLI: no built-in tools (--tools \"\"), no user/project/local settings or hooks (--restricted), no CLAUDE.md, skills, plugins or MCP servers (--safe-mode, --strict-mcp-config), no slash commands, permission requests denied, session not saved.\n" +
		"  - Enforced by the Codex CLI: shell, exec, image, app, plugin, hook, memory and sub-agent tools switched off, web search disabled, user config, rules and project config not loaded, read-only sandbox, no approvals; a run that still shows a tool call is stopped and not recorded.\n" +
		"  - Only requested in the prompt (not enforced): judge the transcript as data and answer with JSON only.\n" +
		"  - The judge CLIs inherit your environment, so settings such as ANTHROPIC_BASE_URL, OPENAI_BASE_URL or a proxy apply to them.\n" +
		"  - drift adds the current CLAUDE.md, CLAUDE.local.md and AGENTS.md files of the project folder (and your global CLAUDE.md) to Claude Code sessions whose log has none; --no-current-instructions turns this off.\n" +
		"  - Every command runs `git rev-parse` (read-only) in project folders found in the logs, to name projects.\n" +
		"  - `report` reads one setting from Claude Code's settings.json, cleanupPeriodDays (how long Claude Code keeps transcripts), and that file's date, to tell deleted logs from no use. It saves each window's cost in numbers only (spend, waste, per-habit amounts, session count; no text) as compare/*.json in the cache, to compare with later when Claude Code has deleted those logs.\n" +
		"  - LEEWAY_JUDGE_CMD, when set, replaces the judge CLIs with a local program (drift says so); LEEWAY_RELEASE_API changes where `self-update --check` asks.\n" +
		"  - Prices come from a local table ({prices}); nothing is fetched.",
	"privacy_paths":         "Cache: {cache}\nReports: {reports}\nSettings: {settings}\nClaude Code logs: {claude}\nCodex logs: {codex}",
	"folder_read":           "Read {tool} logs: {dir} ({n:,} session files, {w:,} touched in this window)",
	"folder_absent":         "No {tool} logs at {dir} (folder not found; {tool} is skipped).",
	"folder_missing":        "The {tool} log folder {dir} does not exist. Check --claude-dir / --codex-dir, settings.json and CLAUDE_CONFIG_DIR / CODEX_HOME.",
	"folder_unreadable":     "The {tool} log folder {dir} cannot be read: {err}",
	"folder_sub_unreadable": "  {n} subfolders could not be read and were skipped.",
	"no_logs":               "No Claude Code or Codex session files were found. If your logs are elsewhere, pass --claude-dir / --codex-dir (or set CLAUDE_CONFIG_DIR / CODEX_HOME as your agents do).",
	"none_in_window":        "No session file was touched in this window; the newest is from {date}. Try a longer --since, e.g. --since 90d.",
	"claude_retention":      "Note: Claude Code deletes transcripts after 30 days by default (cleanupPeriodDays); the oldest one here is from {date}, so the part of the window (or of the comparison window) before it has no Claude Code data.",
	"reading":               "Reading {n:,} log files (the first run takes longer; later runs use the cache) ...",
	"skip_more":             "  ... and {n} more",
	"skip_unreadable_n":     "Skipped {n} log files that could not be opened.",
	"broken_lines":          "Note: {n} log lines were not JSON objects (often a session still being written) and were skipped.",
	"unknown_records":       "Note: {n} log records of a type this version does not know were ignored ({kinds}). A newer agent version may log differently; please report it.",
	"usage_format_changed":  "Warning: {n} Codex logs record token usage only in a newer format this version does not read; their spend is missing from the totals.",
}

// Compare is COMPARE.
var Compare = map[string]string{
	"head":         "Leak per $100 of spend by cause (before -> after). USD at API list prices.",
	"windows":      "Windows: from {a} / from {b} · spend ${ta:,.2f} -> ${tb:,.2f}",
	"line":         "  {x:7.2f} -> {y:7.2f}  ({d:+.2f})  count {na:,} -> {nb:,}  {label}",
	"total":        "  {x:7.2f} -> {y:7.2f}  ({d:+.2f})  total",
	"habits":       "By habit, all sessions (direct and automation): per $100 of spend (before -> after)",
	"lengths":      "Window lengths: {a:.1f} days -> {b:.1f} days",
	"warn_lengths": "Warning: the windows have different lengths; compare the per-$100 rates, not the spend.",
	"warn_overlap": "Warning: the windows overlap, so the same responses count in both.",
	"habit_line":   "  {x:7.2f} -> {y:7.2f}  ({d:+.2f})  {label}",
	"origin":       "{name}: spend ${a:,.2f} -> ${b:,.2f}, leak per $100 {x:.2f} -> {y:.2f} ({d:+.2f})",
	"auto_line":    "    ${a:,.2f} -> ${b:,.2f}  {label}",
	"caveat":       "Note: a change in how you work (kind of task, model, session length) moves these numbers regardless of habits.",
}

// Drift is DRIFT.
var Drift = map[string]string{
	"no_cli":                "Warning: the {tool} CLI was not found; {tool} sessions cannot be checked.",
	"wrong_provider":        "{tool} sessions are judged only by a {tool} model (got {tier}).",
	"lower_tier":            "Warning ({tier}): {msg}",
	"none":                  "No direct-conversation sessions with requests in this window.",
	"head":                  "In-depth check · {n} sessions ({scope}, most recent first{limit}) · rubric {ver}",
	"scope_direct":          "direct conversations",
	"scope_all":             "direct conversations and automations",
	"limit":                 ", at most {n}",
	"resumed":               "{n} sessions already checked are skipped (resuming).",
	"est_unpriced":          "  {tier}: {n} sessions, price unknown (the model has no row in the price table)",
	"est_line":              "  {tier}: {n} sessions, estimated ${usd:.2f}{ref}",
	"preflight_failed":      "Judge CLI check failed, nothing was sent: {msg}",
	"no_cli_any":            "No judge CLI is available for the {n} selected sessions: install and log in to `claude` (Claude Code) or `codex` (Codex), then run this again.",
	"no_cli_skipped":        "  {n} {tool} sessions are left out: the {tool} CLI was not found.",
	"judge_cmd_set":         "Warning: LEEWAY_JUDGE_CMD is set ({cmd}): sessions go to that local program, not to the providers' CLIs.",
	"budget_unpriced":       "The budget cap (${b:.2f}) cannot be enforced for {tier}: its model has no price in the price table. Choose a listed model, add a row to prices.json, or pass --budget 0 to run without a cap.",
	"est_upper":             "  These are upper estimates for the input: nothing is assumed to come from the provider's prompt cache. Judging the same sessions again soon after costs less, because the cache is reused.",
	"plan_line":             "{n} sessions in the window; the {m} most recent not checked yet will be checked (one judge call per session). On a subscription this counts against your plan's usage; with an API key it costs about ${usd:.2f} at list prices.",
	"cut_max":               "Only the {k} most recent of the {total} sessions not checked yet are taken (--max-sessions {k}; ties by session id). Check more with --max-sessions N, or --max-sessions 0 for all {total}.",
	"cut_budget":            "--budget ${b:.2f} (API list-price equivalent) stops the run after about {m} of the {todo} sessions to check, most recent first (ties by session id). Raise it with --budget USD, or leave it out for no dollar limit.",
	"includes_instructions": "Each excerpt includes the instruction files found in the log (CLAUDE.md, AGENTS.md). For Claude Code sessions without them, the current CLAUDE.md, CLAUDE.local.md and AGENTS.md files of the project folder and your global CLAUDE.md are added (--no-current-instructions turns this off).",
	"start_smaller":         "That is a large first run. To start smaller: --max-sessions 10, --budget 2 or --project <name>.",
	"list_line":             "  {date} {tool:11} {project}: about {ktok:,.0f}k tokens ({kb:,.0f} KB)",
	"by_project":            "By project, all {n} sessions to check: {list}",
	"by_project_one":        "{project} {n}",
	"by_project_rest":       "{k} other projects: {n}",
	"list_more":             "  ... and {n} more",
	"inputs_written":        "Wrote the exact judge input of {n} sessions to {dir} (one file per session, plus system-prompt.md). Nothing was sent.",
	"none_judged":           "No session has been checked yet.",
	"est_ref":               " (at least ${ref:.3f} per session: the validation-run average)",
	"est_total":             "  Estimated total ${usd:.2f} at API list prices, billed to whatever account the CLI is logged in with (plan quota on a subscription, money on an API key){budget}",
	"est_budget":            ", budget cap ${b:.2f}",
	"nothing":               "No new sessions to check.",
	"state_on":              "on",
	"state_off":             "off",
	"fable_refused":         "Fable-class models are not used for judging; choose another model.",
	"group_only_conflict":   "--group-only and --no-grouping cannot be used together",
	"group_nothing":         "Grouping, {tool}: nothing to group (fewer than two sessions with problems); no call.",
	"group_unchanged":       "Grouping, {tool}: the judged problems are unchanged since the last grouping; no call.",
	"group_budget":          "Grouping, {tool}: skipped, it would go over the budget of ${b:.2f}.",
	"group_limit":           "Grouping, {tool}: stopped by a usage limit; the report uses the problems one by one.",
	"group_not_run":         "Grouping, {tool}: not run ({msg}).",
	"group_failed":          "Grouping, {tool}: failed ({msg}); the report uses the problems one by one.",
	"group_done":            "Grouping, {tool}: {n} theme(s) across sessions.",
	"group_done_dropped":    "Grouping, {tool}: {n} theme(s) across sessions, {d} dropped by the checks.",
	"stale_prices":          "Note: the price table is {days} days old (read {read}); compare `agent-leeway prices` with the providers' pages, or update agent-leeway.",
	"ended_promo":           "Note: the promotional price of {model} in the price table ended on {date}; check `agent-leeway prices`, or update agent-leeway.",
	"grouping_state":        "Grouping by meaning after judging: {state}. Turn a tool off or on with --grouping claude=off (or settings.json \"grouping\").",
	"grouping_none_on":      "Grouping is off for every judged tool; nothing to do. Turn it on with --grouping claude=on or --grouping codex=on.",
	"by_tool":               "Sessions in the window: Claude Code {claude} · Codex {codex}",
	"left_out":              "{n} {tool} session(s) in the window are not judged in this run (--tool all includes them).",
	"grouping_plan":         "After judging, {n} grouping call(s) ({tools}), about ${usd:.2f} in total: each provider gets only its own sessions' judged problems (the quotes and summaries the judge already saw) and groups them by meaning. --no-grouping skips this.",
	"dry_run":               "--dry-run: estimate only, nothing checked.",
	"scope_change":          "To check fewer or other sessions: --max-sessions N (how many, most recent first), --since 7d (the period), --project <name> (one project).",
	"need_yes":              "Confirmation needed: run in an interactive terminal or add --yes.",
	"ask":                   "Proceed? [y/N] ",
	"cancelled":             "Cancelled.",
	"judged":                "{n} checked, actual cost ${usd:.2f}{reason}",
	"stopped":               " · stopped: {reason}",
	"resume_hint":           "Run the same command again to continue; sessions already judged are skipped.",
	"stop_budget":           "budget cap ${b:.2f} reached",
	"stop_limit":            "the provider answered with a usage-limit response; stopped immediately ({msg})",
	"stop_cancel":           "cancelled (calls that were running are not recorded; rerun to continue)",
	"cli_note":              "  note from the {tool} CLI: {msg}",
	"cli_older":             "{tool} CLI {v} is older than {tested}, the version this was tested with. If checking fails, update it.",
	"debug_on":              "Diagnostics for this run: {path}",
	"debug_at":              "Diagnostics (CLI version, arguments, exit status, error messages; no session text): {path}",
	"debug_hint":            "For a bug report, run the same command again with --debug-log: it writes a diagnostics file without session text.",
	"cli_older_codex":       "Codex CLI {v} is older than {tested} and has no catalog entry for GPT-6.2 Astra. It still uses that model, with default settings. Updating Codex (codex update) gives the setup that was validated.",
	"stop_model":            "the judge CLI refused the model {model}: {msg}",
	"judge_model":           "Expected judging model: {name}",
	"model_cli_default":     "the {tool} default model",
	"model_next":            "The CLI did not accept the model, so the check stopped; the {done} sessions already checked are kept. Next model: {name}. To see what it would check: {cmd}",
	"plan_line_unpriced":    "{n} sessions in the window; the {m} most recent not checked yet will be checked (one judge call per session). On a subscription this counts against your plan's usage; there is no list price for this model, so no dollar estimate.",
	"group_refused":         "Grouping, {tool}: the CLI did not accept {model}; grouping skipped (the checked sessions are kept).",
	"no_model_left":         "No judging model is left to try for {tool} sessions: the CLI refused each one. Choose one with --model (for example --model codex=<model>); refusals are forgotten after 7 days.",
	"judge_model_tool":      "Expected judging model for {tool} sessions: {name}",
	"model_newest":          " (newest)",
	"stop_config":           "the judge CLI could not be run safely: {msg}",
	"stop_fail":             "two judge calls failed in a row; last error: {err} (check the cause, then rerun to continue)",
	"progress":              "  {i}/{n} {tool} {project}: {verdict} (${usd:.3f})",
	"failed":                "  failed: {tier} ({err})",
	"result":                "In-depth check: {n} sessions · {verdicts} · {defects} defect reports · judged by {models}{extra}",
	"result_partial":        " · {n} judged with the middle omitted (too long)",
	"result_custom":         " · custom rubric",
	"see_report":            "Details are in the Markdown/HTML files written by the `report` command.",
	"instr_current_warning": "instructions read from the current files; they may differ from what applied when the session ran",
	"truncated":             "\n...(middle omitted)...\n",
}

// Events is EVENTS.
var Events = map[string]string{
	"model_change":      "model change",
	"effort_change":     "reasoning-effort change",
	"config_update":     "settings change",
	"tools_change":      "tool-list change",
	"shape_change":      "system prompt / tool set change",
	"developer_msg":     "harness instruction",
	"world_state_full":  "environment state resend",
	"settings_applied":  "thread settings applied",
	"new_user_turn":     "new request",
	"subagent_activity": "sub-agent activity",
	"gap_over_5min":     "gap over 5 min",
}

// EventCount is EVENT_COUNT.
var EventCount = "{name} x{n}"

// EventFallback is EVENT_FALLBACK.
var EventFallback = "a change"

// Habits is HABITS (label, text).
var Habits = map[string][2]string{
	"long_break": {"Resuming the same session after a long break", "After a gap longer than the cache lifetime (Claude Code: 5 min or 1 h, Codex: 30 min), leave a summary and start a new session."},
	"mid_switch": {"Switching model, reasoning effort or tools mid-session", "Pick model, reasoning effort, MCP servers and tools when the session starts; switch in a new session."},
	"let_repeat": {"Letting the agent repeat or retry the same command", "Add \"stop and explain after two failures in a row or before running the same command a third time\" to CLAUDE.md / AGENTS.md, and prefer completion signals over polling."},
	"no_map":     {"Not telling the agent where the key files are (it re-reads them)", "Put the key file locations and layout in the first request or in your instructions file."},
	"big_output": {"Letting long logs and outputs into the context whole", "Ask the agent to read only the tail or a summary of test and build logs."},
}

// HourBands is HOUR_BANDS.
var HourBands = []string{"00-06h", "06-12h", "12-18h", "18-24h"}

// Idle is IDLE.
var Idle = map[string]string{
	"title":   "Sessions used in the last {days} days whose prompt cache has probably expired (resuming re-caches the context; USD at list prices)",
	"none":    "  none",
	"showing": "  (showing {n} of {m}, the most expensive first)",
	"line":    "  - {tool:6} {hours:5.1f} h ago · context {ktok:5.0f}k · resuming costs about ${usd:.2f} · {where} · {file}",
	"advice1": "Starting a new session from git state, changed files or a handoff note is cheaper than resuming an expired one (asking it for a summary costs the same as resuming).",
	"advice2": "Next time, before you step away while the cache is warm, ask for \"the conclusions and next steps in handoff.md\".",
	"hook":    "[leeway] {gap:.0f} min since the last response: the prompt cache (lifetime {ttl:.0f} min) has probably expired. This request re-caches about {ktok:.0f}k tokens of context, about ${usd:.2f} at list prices. For a new task, a new session from a handoff note or git state is cheaper.",
}

// Leak is LEAK.
var Leak = map[string]string{
	"title":            "Leak check · {since} – {until} (local time)",
	"only_tool":        " · {tool} only",
	"spend_none":       "Spend in this window $0.00: no responses in this window (the folders read are listed above).",
	"info_uncertain":   "  - Origin unclear for {n:,} sessions (${usd:,.2f} of spend): counted as direct conversations; settings.json \"origin\" can classify their launcher.",
	"info_assumed":     "  - Assumed price: {n:,} responses ({share:.1%} of tokens) of model {model} have no row in the price table and were priced like {fallback}.",
	"spend":            "Spend in this window ${total:,.2f} ({by_tool}); mechanical leak ${leak:,.2f} ({pct:.1%})",
	"prev":             "Previous window of the same length: spend ${total:,.2f}, leak ${leak:,.2f} ({pct:.1%}) -> leak per $100 of spend {a:.2f} -> {b:.2f}",
	"origin_line":      "  {name}: spend ${total:,.2f}, leak ${leak:,.2f} ({pct:.1%})",
	"origin_human":     "Direct conversations",
	"origin_auto":      "Automation runs",
	"origin_note":      "  (automation = Codex automations and goal continuation, worker sessions started by another agent, non-interactive exec, sessions opened by an agent)",
	"habits_title":     "By habit (direct conversations only)",
	"habits_title_all": "By habit",
	"habit_line":       "  - {label}: {n:,} times, ${usd:,.2f} (per $100 of spend {per100:.2f}{prev})",
	"habit_prev":       ", previous window {v:.2f}",
	"habit_where":      "      Where: {wd} {wds:.0%} · {tb} {tbs:.0%} · project {pj} {pjs:.0%} (largest share by amount)",
	"habit_change":     "      Change: {tip}",
	"auto_title":       "Fix in the automation settings (not a personal habit)",
	"auto_line":        "  - {label}: spend ${cost:,.2f}, leak ${leak:,.2f}",
	"causes_title":     "By cause (detail)",
	"causes_split":     "  [direct / automation]",
	"cause_line":       "  ${usd:>9,.2f}  {n:>6,}x  {label}{extra}",
	"cause_split":      "   [${hu:,.2f} {hn:,}x / ${au:,.2f} {an:,}x]",
	"unknown_note":     "  (unexplained rewrites, ${usd:,.2f}, look like server-side cache eviction and are not counted as a habit)",
	"info_title":       "For reference (not counted as leak)",
	"info_subagent":    "  - Sub-agents spent ${usd:,.2f} ({share:.0%} of spend). That is the cost of delegating, not a leak.",
	"info_partial":     "  - Small cache shortfalls ${usd:,.2f} ({n:,} responses, under 20% of the previous context): cache-block granularity, nothing to fix.",
	"sessions_title":   "Sessions with the largest leak (in-window activity dates and amounts)",
	"session_line":     "  - {when}{tag} {tool} {project}: leak ${leak:,.2f} / spend ${cost:,.2f} - {parts}",
	"tag_auto":         " [automation]",
	"tag_mixed":        " [direct+automation {share:.0%}]",
	"started_before":   "{rng} (session started {start}, continued into this window)",
	"keys_note":        "Session keys are in the --json output. Tags: [automation] = most of the session's leak came from automation runs; [direct+automation N%] = N% of it did.",
}

// Limitations is LIMITATIONS.
var Limitations = []string{"Verdicts depend on the judging model: the same session can get different events and verdicts from another model (the model used is named in this report).", "Upper-tier Codex models flag more events than upper-tier Claude models (including on public sessions that are probably fine).", "Claude Code logs do not keep the content of CLAUDE.md, so by default the current instruction files are applied to past sessions. If your instructions changed since, \"instruction violated\" can be wrong.", "Events are found from reactions the user left in the transcript (corrections, refusals, re-requests). Drift nobody reacted to can be missed.", "This is not a score or a ranking; it is material for you to judge with, pointing at locations in the transcript."}

// LowerTierWarning is LOWER_TIER_WARNING.
var LowerTierWarning = "Lower-tier judging models invent citations (quotes not in the transcript) more often. Check the cited locations yourself."

// MoneyNote is MONEY_NOTE.
var MoneyNote = "Amounts are USD at the providers' API list prices (price table read {read}; `agent-leeway prices` shows it). A subscription is not billed per token: read them as list-price equivalents."

// Privacy is PRIVACY.
var Privacy = "Privacy: logs, cache and reports stay on this computer. Two things go out: the in-depth check sends each selected session to its own provider's official CLI (claude -p or codex exec), which sends it wherever that CLI is set up to (its logged-in account, or a base URL or proxy you configured) together with the CLI's own telemetry, and then makes one grouping call per provider with that provider's judged problems (the same quotes the judge already saw, plus its summaries; --no-grouping skips it); and `self-update --check` asks GitHub for the latest release tag. Nothing else opens a network connection (`privacy` lists the details)."

// Report is REPORT.
var Report = map[string]string{
	"title":         "{name} report",
	"meta":          "{now} · {window} · {name} {version}",
	"leak_title":    "Leak check",
	"drift_title":   "In-depth check",
	"drift_empty":   "No judged sessions yet. Run the `drift` command to fill this section.",
	"models":        "- Judging model: {models}",
	"model_item":    "{m} ({n} sessions)",
	"partial":       "- {n} sessions were longer than the judge input limit and were judged with the middle omitted ({k:,}k characters left out).",
	"rubric_custom": " (custom rubric)",
	"rubric":        "- Rubric: {ver}",
	"warning":       "- **Warning**: {msg}",
	"citations":     "- Citation check: {q} quotes, {bad} not found in the transcript; {dropped} events dropped because none of their quotes were found",
	"instr_current": "- {n} Claude sessions were judged with the current instruction files applied to past sessions (if they changed since, \"instruction violated\" may be wrong).",
	"totals":        "{n} sessions: {verdicts}",
	"causes":        "Events by cause: {classes} (detail: {details}) · defect reports (counted separately from events): {defects}",
	"session_head":  "### {verdict} · {tool} · {project} (`{key}`, {tier})",
	"event":         "- {loc} [{cls}: {detail}, {size} drift, {rule}{instr}] {summary}{quote}",
	"event_instr":   ", instruction {loc}",
	"event_quote":   " - evidence: \"{q}\"",
	"defect":        "- {loc} [defect report] {summary}",
	"fixes_title":   "### Sentences to consider adding to your instructions (from \"instruction missing or ambiguous\" events)",
	"fix":           "- ({project}) {s}",
	"limits_title":  "### Known limitations",
	"window":        "last {since}",
	"cli_leak":      "Leak: ${leak:,.2f} of ${total:,.2f} spend ({pct:.1%}) · direct ${h:,.2f}, automation ${a:,.2f}",
	"cli_habit":     "  - {label}: {n:,} times, ${usd:,.2f}",
	"cli_drift":     "Drift: {n} sessions · {verdicts} · {defects} defect reports",
	"cli_no_drift":  "Drift: no judged sessions (run `drift`, or `report --run-drift`)",
	"cli_files":     "Report: {md}\n        {html}",
}

// ShortCause is SHORT_CAUSE.
var ShortCause = map[string]string{
	"expiry":        "cache expired",
	"reset_known":   "rewrite after a change",
	"reset_unknown": "unexplained rewrite",
	"repeat":        "repeated command",
	"fail":          "consecutive failures",
	"reread":        "re-read",
	"oversize":      "oversized output",
}

// Size is SIZE.
var Size = map[string]string{
	"major": "major",
	"minor": "minor",
}

// Tool is TOOL.
var Tool = map[string]string{
	"claude": "Claude Code",
	"codex":  "Codex",
}

// Verdict is VERDICT.
var Verdict = map[string]string{
	"agent_drift":    "Agent drift",
	"prompt_gap":     "Prompt gap",
	"minor_drift":    "Minor drift",
	"as_intended":    "As intended",
	"undeterminable": "Undeterminable",
}

// Weekdays is WEEKDAYS.
var Weekdays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// WindowNote is WINDOW_NOTE.
var WindowNote = "Only responses inside the window are counted; a session that began earlier contributes only its in-window part."
