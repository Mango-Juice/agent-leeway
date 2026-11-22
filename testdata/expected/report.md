# Leeway report

2026-01-20 09:00 +0900 · last 30d · agent-leeway 0.1.0

> Privacy: logs, cache and reports stay on this computer. The only thing sent out is the drift check: each selected session goes to its own provider's official CLI (claude -p or codex exec) for judging.

> Amounts are USD at API list prices (a subscription is not billed this way; read them as list-price equivalents). Codex model prices are not public: GPT-5 list prices are assumed.

## Leak check

```
Leak check · 2025-12-21 09:00 – 2026-01-20 09:00 +0900 (local time)
Amounts are USD at API list prices (a subscription is not billed this way; read them as list-price equivalents). Codex model prices are not public: GPT-5 list prices are assumed.
Only responses inside the window are counted; a session that began earlier contributes only its in-window part.
Spend in this window $4.83 (Claude Code $4.67 · Codex $0.16); mechanical leak $2.38 (49.3%)
Previous window of the same length: spend $0.21, leak $0.02 (9.5%) -> leak per $100 of spend 9.52 -> 49.28
  Direct conversations: spend $4.76, leak $2.37 (49.8%)
  Automation runs: spend $0.08, leak $0.01 (12.5%)
  (automation = Codex automations and goal continuation, worker sessions started by another agent, non-interactive exec, sessions opened by an agent)

By habit (direct conversations only)
  - Letting long logs and outputs into the context whole: 3 times, $1.33 (per $100 of spend 27.99, previous window 0.00)
      Where: Thu 99% · 18-24h 99% · project big logs 99% (largest share by amount)
      Change: Ask the agent to read only the tail or a summary of test and build logs.
  - Switching model, reasoning effort or tools mid-session: 5 times, $0.47 (per $100 of spend 9.91, previous window 0.00)
      Where: Mon 94% · 06-12h 96% · project 데모 프로젝트 94% (largest share by amount)
      Change: Pick model, reasoning effort, MCP servers and tools when the session starts; switch in a new session.
  - Resuming the same session after a long break: 3 times, $0.34 (per $100 of spend 7.22, previous window 0.00)
      Where: Sun 50% · 06-12h 50% · project demo-project 100% (largest share by amount)
      Change: After a long gap (Claude over 1 h, Codex over ~10 min), leave a summary and start a new session.
  - Letting the agent repeat or retry the same command: 10 times, $0.05 (per $100 of spend 0.99, previous window 9.44)
      Where: Sat 40% · 18-24h 52% · project demo-project 40% (largest share by amount)
      Change: Add "stop and explain after two failures in a row or before running the same command a third time" to CLAUDE.md / AGENTS.md, and prefer completion signals over polling.
  - Not telling the agent where the key files are (it re-reads them): 2 times, $0.01 (per $100 of spend 0.23, previous window 0.00)
      Where: Mon 73% · 06-12h 100% · project 데모 프로젝트 73% (largest share by amount)
      Change: Put the key file locations and layout in the first request or in your instructions file.

By cause (detail)  [direct / automation]
  $     0.35       4x  Cache expired (resumed after a long break)   [$0.34 3x / $0.01 1x]
  $     0.47       5x  Cache rewritten mid-session (after a model/settings/tool change)   [$0.47 5x / $0.00 0x]
  $     0.16       1x  Cache rewritten mid-session (cause unknown)   [$0.16 1x / $0.00 0x]
  $     0.02       5x  Same command repeated (3rd run onward)   [$0.02 4x / $0.00 1x]
  $     0.04       8x  Consecutive failures   [$0.03 6x / $0.01 2x]
  $     0.01       2x  Unchanged file read again   [$0.01 2x / $0.00 0x]
  $     1.33       3x  Oversized tool output (over 10k tokens)   [$1.33 3x / $0.00 0x]
  (unexplained rewrites, $0.16, look like server-side cache eviction and are not counted as a habit)

For reference (not counted as leak)
  - Sub-agents spent $0.09 (2% of spend). That is the cost of delegating, not a leak.
  - Small cache shortfalls $0.02 (13 responses, under 20% of the previous context): cache-block granularity, nothing to fix.

Sessions with the largest leak (in-window activity dates and amounts)
  - 2026-01-15 Claude Code big logs: leak $1.32 / spend $2.69 - oversized output $1.32
  - 2026-01-12 Claude Code 데모 프로젝트: leak $0.47 / spend $0.96 - rewrite after a change $0.44
  - 2026-01-10 Claude Code demo-project: leak $0.19 / spend $0.37 - cache expired $0.17, consecutive failures $0.02
  - 2025-12-21 (session started 2025-12-21, continued into this window) Claude Code demo-project: leak $0.17 / spend $0.18 - cache expired $0.17
  - 2026-01-20 Claude Code proyecto-demo: leak $0.17 / spend $0.35 - unexplained rewrite $0.16

Session keys are in the --json output.
```

## Drift check

- Judging model: opus (6 sessions), gpt-6.2-astra (3 sessions)
- Rubric: v2.1-b636272b
- Citation check: 17 quotes, 1 not found in the transcript; 1 events dropped because none of their quotes were found

9 sessions: Agent drift 6, Undeterminable 2, As intended 1

Events by cause: agent-side 7, undeterminable 1, prompt-side 1 (detail: instruction violated 5, agent error 2, undeterminable 1, instruction missing or ambiguous 1) · defect reports (counted separately from events): 1

### Agent drift · Claude Code · 데모 프로젝트 (`000000000104`, claude:opus)
- U3 [agent-side: instruction violated, major drift, R2a, instruction I1] x - evidence: "표는 그대로 두라고 했잖아"
- U1 [prompt-side: instruction missing or ambiguous, minor drift, R2a] x - evidence: "차트를 추가해 줘"
- U2 [defect report] Review command failed to start.

### Agent drift · Codex · 코덱스 (`000000000201`, codex:gpt-6.2-astra)
- U1 [agent-side: agent error, major drift, R2a] x - evidence: "실패하는 테스트를 고쳐"
- U2 [agent-side: agent error, major drift, R2a] x - evidence: "문서도 고쳐"

### Agent drift · Claude Code · proyecto-demo (`000000000103`, claude:opus)
- U2 [agent-side: instruction violated, major drift, R2a, instruction U1] The agent removed a table the user asked to keep. - evidence: "te dije que dejaras la tabla"

### Agent drift · Codex · proyecto (`000000000202`, codex:gpt-6.2-astra)
- U2 [agent-side: instruction violated, major drift, R2a, instruction U1] x - evidence: "te dije que dejaras la tabla"

### Agent drift · Claude Code · demo-project (`000000000101`, claude:opus)
- U2 [agent-side: instruction violated, major drift, R2a, instruction U1] The agent removed a table the user asked to keep. - evidence: "I said keep the table"

### Agent drift · Claude Code · demo-project (`000000000102`, claude:opus)
- U2 [agent-side: instruction violated, major drift, R2a, instruction U1] The agent removed a table the user asked to keep. - evidence: "표는 그대로 두라고 했잖아"

### Undeterminable · Claude Code · panel (`000000000107`, claude:opus)
- U1 [undeterminable: undeterminable, major drift, R2a] x - evidence: "Rename the helper"

### Sentences to consider adding to your instructions (from "instruction missing or ambiguous" events)

- (데모 프로젝트) 표는 유지하고 차트만 추가해 줘.

### Known limitations

- Verdicts depend on the judging model: the same session can get different events and verdicts from another model (the model used is named in this report).
- Upper-tier Codex models flag more events than upper-tier Claude models (including on public sessions that are probably fine).
- Claude Code logs do not keep the content of CLAUDE.md, so by default the current instruction files are applied to past sessions. If your instructions changed since, "instruction violated" can be wrong.
- Events are found from reactions the user left in the transcript (corrections, refusals, re-requests). Drift nobody reacted to can be missed.
- This is not a score or a ranking; it is material for you to judge with, pointing at locations in the transcript.
