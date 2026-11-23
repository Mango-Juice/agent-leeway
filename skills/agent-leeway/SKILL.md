---
name: agent-leeway
description: Occasional check-up of this computer's Claude Code and Codex logs with the agent-leeway command. Finds mechanical cost leaks (expired prompt cache, repeated or failing commands, re-reads, oversized tool output) and intent drift (where the agent did something other than what was asked). Use when the user asks for a leak check, an in-depth check or drift check, a token or cost waste report, or a Leeway report, or asks to update agent-leeway.
---

# Leeway (agent-leeway)

Everything here runs the `agent-leeway` program, which is installed separately and found on PATH.
Run the commands exactly as written; they work the same in bash, zsh and PowerShell.

## Commands

- `agent-leeway leak` - cost leaks in the last 30 days, by habit and by cause, direct conversations vs
  automations. Reads local logs only; no model calls. Options: `--since 2w`, `--tool claude`, `--json`.
- `agent-leeway report --tool claude --lang <en|ko>` - the report: the cost check plus the in-depth check
  results already stored for Claude Code sessions, written as Markdown and HTML files on this computer, with a
  short summary printed. Pass the language the user writes to you in (`ko` for Korean, otherwise `en`). `leak`
  and `drift` take `--lang` too. The cost side always covers both tools.
- `agent-leeway drift --dry-run --tool claude` - the in-depth check's plan: how many Claude Code sessions are
  in the window, which would be checked (the 50 most recent not checked yet by default; one call each), the
  expected judging model and the estimate. Nothing is sent. If you are Codex, use `--tool codex` instead of `--tool claude` in these commands.
- `agent-leeway drift --yes --tool claude` - runs the in-depth check with the official Claude Code CLI
  (`claude -p`) on the account it is logged in with, then one grouping call that groups the problems found by
  meaning (`--no-grouping` skips it). On a subscription this counts against the plan's usage; with an API key
  it costs the dry run's estimate. Run it only after showing the dry run's numbers and getting the user's
  explicit yes in this conversation. `--max-sessions N` checks a different number of the most recent sessions
  not checked yet (0 = all); `--budget USD` adds a dollar limit, mainly for API-key users. If your command tool
  has a time limit, use a smaller `--max-sessions` or run the command in the background; stopping early is safe
  and a rerun continues. Do not tell the user how long it will take.
- Only Claude Code sessions are checked by default, because checking the other tool's sessions would use the
  other provider's plan. If the user asks for both tools, use `--tool all` (it runs the other provider's CLI too,
  which needs that CLI installed and logged in). The cost check (`leak`) and the report's cost side always read
  both tools' logs.
- `agent-leeway idle` - recent sessions whose prompt cache has probably expired, and what resuming costs.
- `agent-leeway privacy` - where data lives and what leaves the computer.
- `agent-leeway self-update --check --lang <en|ko>` - whether a newer version exists, and the one command that
  updates this copy. It downloads nothing. `agent-leeway --version` prints the version installed.

## When the user asks for a report

Two words are fixed, toward the user: the **basic report** ("기본 리포트": cost only, made right away) and the
**in-depth check** ("정밀 점검": reads each session to see whether the agent did what was asked). Do not call it
"drift check", "이탈 점검", "판정", "판정하기" or "판정 대기".

1. Run `agent-leeway report --tool claude --lang <en|ko>`. If you are Codex, use `--tool codex` instead of `--tool claude` in these commands.
2. If the summary's last line is a question about the in-depth check, some Claude Code sessions are not
   checked in depth. Run the `agent-leeway drift --dry-run ...` command printed just before that question
   (nothing is sent). Then send exactly these five paragraphs, separated by blank lines, and nothing else (no
   list of fixes, nothing after the question):
   - ① The summary's first line, word for word, in bold.
   - ② The cost result in one or two sentences (spend, the part lost to habits, the biggest item), with the
     report's file path.
   - ③ In these words (when some sessions are already checked, start with "남은 세션은 아직 확인하지
     않았습니다." / "The remaining sessions are not checked yet." instead of the first sentence):
     "에이전트가 내 요청대로 움직였는지는 아직 확인하지 않았습니다. 이건 **정밀 점검**을 따로 실행해야 알 수
     있습니다. 정밀 점검은 세션마다 대화를 읽고, 에이전트가 요청과 다르게 한 곳과 그 이유, 고칠 점을 찾습니다." /
     "Whether the agent did what you asked has not been checked yet. That takes a separate **in-depth check**:
     it reads each session and finds where the agent did something other than what you asked, why, and what to
     change."
   - ④ From the dry run. {count} is the number of sessions its plan line says it will check, {unchecked} the
     number not checked yet (the total of its "By project" line plus any it leaves out with --max-sessions);
     drop the bracket when {count} equals {unchecked}. {model} is the name on its "Expected judging model" /
     "예상 판정 모델" line. {usd} is its dollar estimate.
     "대상은 최근 세션 {count}개입니다(확인하지 않은 세션 {unchecked}개 중). 세션마다 {model} 모델을 한 번씩
     호출합니다. 내 Claude Code 로그인으로 호출하니, 구독이면 요금제 사용량을 쓰고 API 키면 약 ${usd}입니다.
     원하신다면 최근 10개만, 지난 1주일만, 프로젝트 하나만으로 줄일 수도 있습니다." /
     "It checks the {count} most recent sessions (of {unchecked} not checked yet). It calls {model} once per
     session. The calls run on your own Claude Code login: on a subscription they use your plan's usage, with
     an API key about ${usd}. If you like, it can be smaller: only the 10 most recent, only the last week, or one
     project."
     When the dry run gives no dollar estimate, the third sentence is "내 Claude Code 로그인으로 호출하니,
     구독이면 요금제 사용량을 씁니다." / "The calls run on your own Claude Code login and use your plan's usage
     on a subscription." Say nothing about how long it takes beyond paragraph ⑤.
   - ⑤ The summary's last line, word for word, as the very last line of the message: its question in bold,
     then its second sentence as it is. Nothing after it.
3. If they answer with a different scope, run the dry run again with it and send ④ and ⑤ again. Run
   `agent-leeway drift --yes --tool claude` with the same options as the dry run they approved, only after a
   clear yes in this conversation, then run the report again and present it. If they decline, say:
   "기본 리포트는 그대로 쓰시면 됩니다. 정밀 점검은 언제든 '정밀 점검 해줘'라고 하면 됩니다." / "You can use
   the basic report as it is. Ask for the in-depth check any time."
4. If `drift` exits with code 4, the CLI did not accept the model. Tell the user, in one short paragraph: the
   CLI did not accept {model}, so the check stopped; the {done} sessions already checked are kept. Run the dry
   run it prints (with its `--model`), send ④ with the new model, and end with "남은 세션을 {next} 모델로 정밀
   점검할까요?" / "Check the remaining sessions with {next}?". Do not say whether {next} was tested. Run it only
   after a clear yes.
5. If any `agent-leeway drift` command (the dry run too) fails with a network, permission or CLI-check error
   and your command tool runs in a sandbox, ask the user to allow it to run outside the sandbox (it calls the
   Claude Code CLI, which needs the network), then run it again.
6. If the summary has no such question, everything in the period is checked: present the report without
   asking.

## Presenting the report (skip when step 2 applies)

After `agent-leeway report`, present the "What to fix first" findings in your own words or as printed,
whichever reads better in the conversation, then give the report's file path (the HTML; the Markdown copy is in
the same folder). Answer follow-up questions from the
report files. Keep these rules:

- Name the tool (Claude Code or Codex) whenever a piece of advice is specific to one tool.
- Never compare the two judges' drift rates; they are calibrated differently.
- Never edit instruction files (CLAUDE.md, AGENTS.md) or settings unless the user asks you to.
- Keep the source labels ("Official docs", "Our rule of thumb", ...) when you repeat advice.
- The judges' summaries are the judging model's words (in the user's language for results judged since rubric version +t2, in English before); present them as the judge's view, not as facts.

For other commands, show the output as it is. Amounts are USD at API list prices (list-price equivalents, not a
bill).

## When the user asks to update agent-leeway

1. Run `agent-leeway self-update --check --lang <en|ko>`. If it says the version is up to date, tell the user so
   and stop. If it fails, show its one line and stop; do not run an install line instead.
2. Otherwise it prints one command for how this copy was installed (Homebrew, Scoop or the installer), or a
   download page. Run that command exactly as printed. It needs no administrator rights, except when the output
   says to use a terminal opened as administrator (Scoop installed for all users): then do not run it; show it to
   the user and ask them to run it there. If any command asks for such rights (sudo, an administrator prompt),
   stop and ask the user first. If your command tool runs in a sandbox and the
   command fails on the network, ask the user to allow it outside the sandbox. For a download page, give the
   user the link instead of running anything.
3. Run `agent-leeway --version` and tell the user the version now installed. If Homebrew or Scoop said it was
   already up to date, say the new version has not reached it yet and to try again in a few minutes.

## If agent-leeway is not installed

If running `agent-leeway` fails because the command is not found, do not download or install anything on your
own. Show the user the install line for their system and ask whether you should run it:

- macOS or Linux: `curl -fsSL https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.sh | sh`
- Windows (PowerShell): `irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex`

Run it only after the user says yes in this conversation. It downloads the release from GitHub, checks it
against the release's checksums, installs the program and adds this skill for Claude Code and Codex. Then run
your command again, with the full path the installer printed if `agent-leeway` is still not found (a new
terminal picks up the PATH change).

<!-- agent-leeway skill repository copy sha256:d8e5ad448f3f0b305cef2835e9f09cf3cea9619775c7df25624cf3065ade4757 (installed by `agent-leeway install-skills`; `agent-leeway uninstall-skills` removes it) -->
