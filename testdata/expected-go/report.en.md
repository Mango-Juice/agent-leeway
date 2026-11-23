# Your AI coding agents, overall: Poor

2025-12-21 – 2026-01-20

| | Rating (good · fair · poor) | Details |
|:--|:--|:--|
| Did what you asked | **Poor** | Serious misses in 6 of 9 sessions · 6 fewer missed sessions to reach Good |
| Money wasted by habits | **Poor** | $2.55 of $5.36 (48%) · $2.28 less to reach Good |

> **!** Checked with your edited rubric v2.1-b636272b+t2; the validation figures in this report were measured with the bundled rubric v2.1-cf92779d

### Fix first: Add what you had to repeat to CLAUDE.md or AGENTS.md

In 2 sessions: “deja la tabla como está”

[Claude Code docs](https://code.claude.com/docs/en/memory.md) · [Codex docs](https://learn.chatgpt.com/docs/agent-configuration/agents-md) · **→ [Show what happened](#f-drift-broke-chat)**

Then: [Keep long logs out](#f-leak-big-output) · [Make broken rules checkable](#f-drift-broke-file)

---

<a id="f-drift-broke-chat"></a>
## Fix 1 of 3: Add what you had to repeat to CLAUDE.md or AGENTS.md

**You had to say this again in 2 sessions.**

> You had said: “deja la tabla como está”

Claude Code and Codex read this file at the start of every session. [Claude Code docs](https://code.claude.com/docs/en/memory.md) · [Codex docs](https://learn.chatgpt.com/docs/agent-configuration/agents-md)

_How sure: Firm_

Draft line for CLAUDE.md or AGENTS.md, from your own words. Edit it before you paste.

```text
deja la tabla como está
```

**The agent went against something you had already said.**

> You, earlier: “deja la tabla como está”  
> What the agent did: The agent removed a table the user asked to keep.  
> You, then: “te dije que dejaras la tabla”

→ Fixed in the same session, after you pointed it out.

**The agent went against something you had already said.**

> You, earlier: “deja la tabla como está”  
> What the agent did: x  
> You, then: “te dije que dejaras la tabla”

→ Fixed in the same session, after you pointed it out.

<a id="f-leak-big-output"></a>
## Fix 2 of 3: Keep long logs out of the chat

**Long outputs in the chat cost $1.33, 3 times. One session alone cost $1.32.**

Models get less accurate as the input gets longer. [Source](https://www.trychroma.com/research/context-rot)

_How sure: Measured_

<a id="f-drift-broke-file"></a>
## Fix 3 of 3: Make the rules it broke specific enough to check

**1 rule was broken, in 1 session. For example:**

> Rule in your CLAUDE.md: “표는 지우지 말고 그대로 유지할 것”

The docs say adding IMPORTANT probably won't fix an ignored rule. A rule it can check, or a hook, works better. [Source](https://code.claude.com/docs/en/best-practices.md)

_How sure: Likely_

**The agent broke a rule from your CLAUDE.md.**

> Rule in your CLAUDE.md: “표는 지우지 말고 그대로 유지할 것”  
> What the agent did: x  
> You, then: “표는 그대로 두라고 했잖아”

→ Fixed in the same session, after you pointed it out.

## What is going well

- **9 / 9** problems were fixed within the same session

## Where it went off, and where the money went

### Where the agent went off

Most often: Ignored something you had said, in 4 of 9 sessions.

| | Details |
|:--|--:|
| Ignored something you had said | 4 of 9 |
| Broke a rule in your instruction file | 1 of 9 |
| A mistake you had to point out | 1 of 9 |

_Each bar counts the checked sessions that had this problem at least once, big or small. One session can have several problems, so the bars add up to more than the number of sessions._

"A mistake you had to point out" (1 session) is not one of the three fixes at the top of this report, because its advice is general. You can find it under Look things up.

### Where the money went

Long outputs in the chat cost the most: $1.33. Habits wasted $2.55 in all, out of $5.36 spent.

| | Details |
|:--|--:|
| Long outputs in the chat | $1.33 (52%) |
| Switching model mid-session | $0.58 (23%) |
| Resuming after long breaks | $0.35 (14%) |
| Repeating failing commands | $0.10 (3.9%) |
| Rereading the same files | $0.02 (0.8%) |

_Each bar is a share of the $2.55 lost to habits. Other is cost whose cause the logs do not record._

### How each tool's judge rated its sessions

- Claude Code · judged by Claude Opus 5.5: Serious miss 4 · Could not check 2
- Codex · judged by GPT-6.2 Astra: Serious miss 2 · As asked 1

_The two judges are calibrated differently. Read each bar on its own._

Not checked in depth yet: 2 Claude Code sessions in this period. To see what would be checked (nothing is sent): agent-leeway drift --dry-run --tool claude --since 30d

## Look things up

The rest of the report, in full.

### Other things worth fixing

<a id="f-drift-mistake"></a>
#### Stop it early, and start over after two failed fixes

**You had to fix its own mistakes in 1 session.** In Codex you can press Enter while it works to steer the current turn. [Codex docs](https://learn.chatgpt.com/docs/prompting)

_How sure: Check this · found only by the Codex judge, which flags more_

**The agent made a mistake, and you had to point it out.**

> What the agent did: x  
> You, then: “실패하는 테스트를 고쳐”

→ Fixed in the same session, after you pointed it out.

<a id="f-leak-long-break"></a>
#### Start a new session after a long break

**Picking up old sessions cost $0.35, 4 times. One session alone cost $0.17.** After a break the cache is gone, and the whole chat is billed again. [Claude Code docs](https://code.claude.com/docs/en/prompt-caching.md)

_How sure: Measured_

<a id="f-leak-mid-switch"></a>
#### Pick the model before you start

**Switching mid-session cost $0.58, 5 times. One session alone cost $0.44.** Each model has its own cache, so a switch rereads the whole chat. [Claude Code docs](https://code.claude.com/docs/en/prompt-caching.md) · [Leeway measurement](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)

_How sure: Measured_

<a id="f-leak-let-repeat"></a>
#### Tell the agent to stop after two failures

**Repeated and failing commands cost $0.10, 13 times. One session alone cost $0.04.** This is our own measurement of your logs. [Leeway measurement](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)

_How sure: Measured_

`All projects · ~/.claude/CLAUDE.md`

```text
Stop and report the cause after two failures in a row, or before running the same command a third time unchanged.
```

`All projects · ~/.codex/AGENTS.md`

```text
Stop and report the cause after two failures in a row, or before running the same command a third time unchanged.
```

<a id="f-drift-prompt"></a>
#### Say what done looks like before you start

**In 1 of 9 sessions the request left out something you wanted.** Specific instructions get followed more reliably than vague ones. [Claude Code docs](https://code.claude.com/docs/en/memory.md)

_How sure: Check this · judges detect this cause least; there are probably more_

**The request left out something you wanted.**

> What the agent did: x  
> You, then: “차트를 추가해 줘”

→ Fixed in the same session, after you pointed it out.

`데모 프로젝트 · CLAUDE.md`

```text
표는 유지하고 차트만 추가해 줘.
```

### Sessions with problems

7 of 9 checked sessions had at least one problem. The worst come first.

#### Serious miss · 데모 프로젝트 · `000000000104` · Claude Code

- U3 · you corrected it · undone or redone: “표는 그대로 두라고 했잖아”
  - Judge's summary: x
- U1 · you corrected it · small fix: “차트를 추가해 줘”
  - Judge's summary: x

#### Serious miss · 코덱스 · `000000000201` · Codex

- U1 · you corrected it · undone or redone: “실패하는 테스트를 고쳐”
  - Judge's summary: x
- U2 · you corrected it · undone or redone: “문서도 고쳐”
  - Judge's summary: x

#### Serious miss · proyecto-demo · `000000000103` · Claude Code

- U2 · you corrected it · undone or redone: “te dije que dejaras la tabla”
  - Judge's summary: The agent removed a table the user asked to keep.

#### Serious miss · proyecto · `000000000202` · Codex

- U2 · you corrected it · undone or redone: “te dije que dejaras la tabla”
  - Judge's summary: x

#### Serious miss · demo-project · `000000000101` · Claude Code

- U2 · you corrected it · undone or redone: “I said keep the table”
  - Judge's summary: The agent removed a table the user asked to keep.

#### Serious miss · demo-project · `000000000102` · Claude Code

- U2 · you corrected it · undone or redone: “표는 그대로 두라고 했잖아”
  - Judge's summary: The agent removed a table the user asked to keep.

#### Could not check · panel · `000000000107` · Claude Code · origin unclear

- U1 · you corrected it · undone or redone: “Rename the helper”
  - Judge's summary: x

### How we checked

- **What we read**: Claude Code and Codex logs on this computer, 2025-12-21 to 2026-01-20. 13 sessions.
- **How sessions were checked**: Each tool's own model read each session against a fixed rubric (v2.1-b636272b+t2). The two judges are calibrated differently, so their results are kept apart.
- **Where the dollars come from**: API list prices, checked 2026-11-22. A subscription is not billed this way.
- **Privacy**: This page was made on this computer. Nothing was sent anywhere to build it.
- Checked with your edited rubric v2.1-b636272b+t2; the validation figures in this report were measured with the bundled rubric v2.1-cf92779d
- 2 sessions could not be told apart as your own conversation or an automation; they are counted as your own.
- Judge programs: LEEWAY_JUDGE_CMD.
- Citation check: 17 quotes checked, 1 not found; 1 event dropped because none of their quotes was found.
- Not checked in depth yet: 2 Claude Code sessions in this period. To see what would be checked (nothing is sent): agent-leeway drift --dry-run --tool claude --since 30d

[Full method and known limits](https://github.com/Mango-Juice/agent-leeway/blob/main/docs/validation.md)
