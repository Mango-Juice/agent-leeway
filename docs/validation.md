# Validation summary

This page summarizes how Leeway was checked before release. It has two parts:
- **Public data**: results that can be reproduced from public data.
- **The developer's own logs**: the logs of **one developer**. Only aggregate numbers are reported. No transcripts, project names or per-session rows are published.

The experiments were pre-registered: the pass criteria were written down before the diagnoser's outputs were seen. None of the checks below compares the tool against **human** labels directly.

> **Which rubric.** §1.1–§1.5 report runs with **rubric v2, written in Korean**. The canonical rubric is now **v2.1 in English** ([rubric.md](rubric.md)). It differs from v2 in three ways at once: language, wording, and a fixed cause procedure. On public data, v2.1 passes P1, P2 and P4 (§1.6). With the Go tool it passes P3 reproducibility on 15 sessions per model (§1.7). Every pre-registered quality criterion of the publication rule (P1, P3, P4) has passed for v2.1. A Korean translation of v2.1 judges like the English text at the level of run-to-run noise (§1.8). The judge's summaries are now written in the user's language; that change was re-checked against run-to-run noise (§1.9). The grouping step that runs after judging is checked in §1.10.

> **Rubric version ids.** The judge sees only Part B of the rubric and the settings rows S1–S10. The id shown by `agent-leeway rubric` is a hash of the whole file, so it changes when the explanatory parts change.
> - Validated: v2.1-3fbcc065 (§1.6) and v2.1-b636272b (§1.7). The two files differed only in Part A: the first still had the product-name placeholders that are filled in when the files are published.
> - v2.1-cf92779d: the same judged text, after Part C was updated with these results. The judged text (system prompt built from Part B and S1–S10) is byte-identical in these three; a test checks this against the validated file.
> - Shipped: **v2.1-cf92779d+t2**. One change to the judged text: the judge's free-text summaries and the verdict reason are written in the language the user mainly writes in, instead of English. Field names, codes and quotes are unchanged. Re-checked in §1.9.

> **Which program produced these numbers.** The reproducibility run (§1.7, 60 judge calls), the summary-language re-check (§1.9) and the grouping checks (§1.10) were made by the released Go program against the real `claude` and `codex` CLIs. All other judged runs were made by the Python research implementation, of which the Go program is a port. The port was checked for exact parity: all 339 cached v2 judgments replayed with 0 verdict mismatches, transcripts re-rendered from the logs were byte-identical, and the citation checks were equal ([development.md](development.md)).

## 1. Drift diagnosis (§1.1–1.5: rubric v2, Korean; §1.6: rubric v2.1, English)

**Setup**
- **Diagnoser**: one call per session. The machine part of the rubric (Part B) is placed in the prompt together with the user's settings. The model returns drift events, causes, size and a session verdict as JSON, with a verbatim quote for every event.
- **Checks in code**:
  - every quote must appear verbatim at the cited location
  - the session verdict is recomputed from the event list
  - schema violations are counted
- **Models**:
  - Claude Code sessions: Claude Opus 5.5, through the official `claude` CLI
  - Codex sessions: gpt-6.2-astra (reasoning medium), through the official `codex` CLI
  - Each session went only to its own provider.

### 1.1 Public data
- **Sessions**: 80 SpecStory session histories from public GitHub repositories under permissive licenses.
  - 40 have at least one validated misalignment episode in the replication package of Tang et al. (2026).
  - 40 come from the same repositories and have no episode. They are only *presumed* negative, because the original extractor favored precision.
- **Reference labels**: produced by an LLM (GPT-5.4). In the paper's human audit, extraction precision was 0.93 and cause accuracy 0.72. These are **model labels with a human audit**, not human labels.

| Measure | Opus 5.5 | gpt-6.2-astra | Pass bar |
|---|---|---|---|
| Recall of user-visible episodes (±1 user turn) | 0.67 [0.57, 0.77] | 0.87 [0.80, 0.94] | ≥ 0.6 |
| Same recall, also counting defect reports | 0.91 | 0.95 | — |
| Sessions with ≥ 1 event: positive vs presumed negative | 78% vs 36% | 93% vs 55% | — |
| AUC (events per session, positive vs presumed negative) | 0.72 [0.61, 0.83] | 0.74 [0.63, 0.84] | ≥ 0.65 |
| Sessions with a defect report | 48% | 51% | — |
| Events whose cause is "agent error" | 35% | 39% | — |

Brackets are bootstrap 95% intervals (sessions resampled 2,000 times).

### 1.2 The developer's own logs (one developer, aggregates only)
- **Sample**: 120 direct (non-automated) sessions, 40 Claude Code and 80 Codex.
- **Weighting**: sessions with strong drift signals were over-sampled, then re-weighted.

| Measure | Opus 5.5 | gpt-6.2-astra | Pass bar |
|---|---|---|---|
| Verdict agreement on re-run (30 sessions) | 0.97 | 0.97 | ≥ 0.8 |
| Event-location overlap on re-run (Jaccard) | 0.86 | 0.93 | ≥ 0.6 |
| Quotes not found in the transcript (all runs) | 0.1% (1 of 684) | 0.2% (3 of 1,903) | ≤ 5% |
| Weighted κ vs agent proxy labels (v1 rubric, not human; reference only) | 0.56 | 0.41 | ≥ 0.4 |
| Model verdict = verdict recomputed in code | 0.99–1.00 | 0.94–0.99 | — |
| Events whose cause is "agent error" | 63% | 29% | — |
| Sessions judged as a major drift (proxy labels: 8) | 4 | 28 | — |

These figures are for the top-tier judges above. In the v1 run, lower-tier judges quoted text that was not in the transcript far more often: one model did so in 23% of quotes. So the results here say nothing about lower-tier judges. The tool warns when one is chosen.

**What a drift check uses.** One judge call per session, through the CLI the user is logged in with. On a subscription this counts against the plan's usage. With an API key it costs roughly $0.15 to $0.35 per session at list price. These are approximate figures measured on the validation runs: Claude Opus $0.24 (40 sessions) and $0.32 (15 sessions); gpt-6.2-astra $0.21 (80 sessions) and $0.15 (15 sessions). Judging the same sessions again within the provider's cache lifetime costs much less: $0.05 per session for Opus. Prices come from the providers' price pages; `agent-leeway prices` shows them and `prices --edit` updates them. The tool shows the session count and an estimate and asks before any call; `--max-sessions` and `--budget` limit a run.

### 1.3 Model differences in verdicts
- In the v1 cross-check, Opus and astra judged the same 22 sessions. Agreement was weighted κ 0.61 for the verdict and κ 0.52 for the cause.
- In the v2 cross-check, both models judged the same 44 developer sessions (31 Codex, 13 Claude Code; 2 further Claude Code calls failed).
  - Verdict agreement: 0.73, weighted κ 0.61, the same κ as in v1.
  - Cause agreement on matched events: 0.68 (κ 0.52, 80 pairs) for the four causes, 0.88 (κ 0.71) for the three groups.
  - Most four-cause disagreements were again instruction violation versus agent error (16 of 26).

### 1.4 Cause attribution: why the "agent error" share differed, and the change in v2.1
- **The difference is between sessions, not models (confirmed with rubric v2).** The 63% and 29% above come from *different* sessions: Opus judged the Claude Code sessions, astra the Codex sessions.
  - On the same 44 developer sessions, the two models were within 3–4 points of each other:
    - Codex sessions: agent error 34% (Opus) vs 31% (astra)
    - Claude Code sessions: agent error 64% (Opus) vs 60% (astra)
  - On the same 79 public sessions the distributions were also close:
    - instruction violation: 33% vs 35%
    - missing or ambiguous instruction: 23% vs 24%
    - agent error: 35% vs 39%
  - The likely reason: most Codex sessions carried long instruction files in their logs, while the Claude Code sessions only had short current files. With few explicit instructions to break, an agent-side event lands on "agent error" rather than "instruction violation".
  - Limits of this check:
    - Only 13 Claude Code sessions.
    - It used rubric v2, not v2.1.
    - Each model judged the other tool's log style, which it had not seen before.
- **Where the models disagreed.** 72 event pairs were matched on the same public sessions. The four causes agreed in 0.71 of pairs (κ 0.59).
  - Of 16 genuine disagreements, 10 were instruction violation versus agent error. All 10 came from two situations: answering a question by acting, and reporting success without checking.
  - One model quoted the user's own request (the one in which the event happened) as the "broken instruction".
- **Three cause groups are more stable.** Grouped as agent-side / prompt-side / undeterminable:
  - agreement between models rose from 0.71 to **0.83** (κ 0.65)
  - agreement with the public labels rose from 0.46–0.57 to **0.69–0.74**
- **"Missing or ambiguous instruction" is the weakest cause.** Agreement with the public labels' "underspecified instruction" was 0.43 (Opus) and 0.17 (astra).
- **Changes in v2.1:**
  - a fixed cause order with evidence requirements: a violation needs an instruction from *before* the event
  - agent error is the default for those two situations
  - reports lead with the three groups

  Measured results are in §1.6. Not all of them went the expected way.

### 1.5 Known weaknesses shown by these checks
- **No direct human validation.** The public reference labels come from a model with a human audit. The developer-log comparison used agent proxy labels.
- **Verdicts depend on the model** (§1.3). The report always names the model used.
- **The Codex judge (astra) over-calls.**
  - It found events in 55% of presumed-negative public sessions with rubric v2, and in **60%** with v2.1. Opus: 36% with both.
  - It flagged 28 major drifts on the developer sample (v2), against 8 in the proxy labels.
- **Instruction files of past Claude Code sessions are not in their logs.** An option reads the *current* files, which may differ from what applied at the time.
- **Circularity.** Opus is in the same model family as the proxy labeler; astra is in the same family as the public label model (GPT-5.4). The cross-family pairs give the more conservative estimates.
- **One developer.** Rubric v2 was refined on the same developer's sessions, so results on that sample may be optimistic. Public-data results take priority.
- **Silent drift cannot be checked on public data.** This covers hand edits and restarts elsewhere. The public labels exclude it.
- **Compressed Codex logs were missed during development.** Codex compresses older session files (`*.jsonl.zst`). The research pipeline read only uncompressed files, so the developer-log figures **under-count** Codex activity. Most of one month and part of two others were missed.
  - Leak totals for the same 30-day window rose by about 29% once compressed files were read.
  - The drift sample was drawn from a population that lacked those sessions.
  - The verdicts recorded for sampled sessions are unaffected.
  - The released Go program reads compressed logs (and Codex `archived_sessions`).

### 1.6 Rubric v2.1 on public data
- **Setup.** The same 80 public sessions, each judged once by Opus 5.5 and once by gpt-6.2-astra, with the English rubric v2.1.
- **Calls.** Opus succeeded on 79 (one safeguard refusal of the same session as in v1 and v2). astra succeeded on all 80.
- **Pre-registration.** The plan, the criteria and the expected direction of each change were committed before the first call.

| Criterion | v2.1 | v2 | Result |
|---|---|---|---|
| P1 episode recall (±1 user turn), ≥ 0.6 | Opus 0.67 [0.57, 0.77], astra 0.88 [0.81, 0.94] (85 episodes) | 0.67 / 0.87 | pass |
| P2 sessions with ≥ 1 event, positive vs presumed negative, and AUC ≥ 0.65 | Opus 75% vs 36%, AUC 0.71 [0.60, 0.82]; astra 90% vs 60%, AUC 0.74 [0.63, 0.84] | 0.72 / 0.74 | pass |
| P3 reproducibility | see §1.7 (developer logs, Go tool) | 0.97 / 0.97 | pass (§1.7) |
| P4 quotes not found in the transcript, ≤ 5% | Opus 0.3% (1 of 327), astra 0.2% (1 of 644); 0 schema problems; model verdict = code verdict in 100% of runs | 0.1% / 0.2% | pass |
| P5 proxy labels | **not run** | 0.56 / 0.41 | not judged |

**What changed in cause attribution** (v2 → v2.1, same public sessions; matched events within ±1 user turn):

| Measure | v2 | v2.1 | Expected | Outcome |
|---|---|---|---|---|
| Four-cause agreement between the two models | 0.71, κ 0.59 (72 pairs) | 0.80, κ 0.70 (69 pairs) | higher | as expected |
| Three-group agreement between the two models | 0.83, κ 0.65 | 0.87, κ 0.69 | higher or equal | as expected |
| Agreement with the public labels' cause, four causes | Opus 0.57 (42), astra 0.46 (54) | Opus 0.49 (41), astra 0.44 (54) | higher | **not met: lower for both** |
| Same, three groups | Opus 0.74, astra 0.69 | Opus 0.78, astra 0.74 | higher or equal | as expected |
| Recall of the public "missing or ambiguous instruction" label | Opus 0.43 (7), astra 0.17 (12) | Opus 0.33 (6), astra 0.23 (13) | higher | **not met: Opus lower, astra higher; counts too small to conclude** |
| Answering-by-acting and unverified-completion events judged "agent error" (the new default) | Opus 0.71, astra 0.60 | Opus 0.96, astra 0.98 | majority | as expected |
| "Instruction violation" citing an instruction at or after the event (forbidden in v2.1) | not checked | 0 of 58 | near 0 | as expected |
| astra: presumed-negative sessions with any event | 0.55 | 0.60 | lower or equal | **not met: higher** (Opus 0.36 in both) |

**How to read this**
- **The two judges agree with each other more**, and they follow the new cause rules almost without exception.
- **Agreement with the public cause labels fell on the four-cause scale.** The new default moves events that the public labels call a failure to follow instructions into "agent error". The public label scheme has no agent-error class. On the three-group scale, where both count as agent-side, agreement rose.
- **"Missing or ambiguous instruction" remains the weakest cause** (0.23–0.33 recall), on small counts (6 and 13 episodes).
- **astra's over-calling did not improve** (55% → 60%).
- **Confound:** language (Korean → English), wording and the cause procedure changed together. §1.8 separates the language from the other two.

### 1.7 Rubric v2.1 reproducibility with the Go tool (P3)
- **Setup.**
  - Developer logs: 15 Claude Code sessions judged by Opus 5.5 and 15 Codex sessions judged by gpt-6.2-astra.
  - Each session was judged twice by the released Go program, from a frozen copy of the logs, with separate caches.
  - The sessions were drawn with a fixed seed from the v2 reproducibility pools. Those pools came from a sample over-weighted toward drift signals.
- **Calls.** 60 calls; all 60 accepted every lock-down flag, stayed one turn with no tool use, and parsed with 0 schema problems and 0 ungrounded events.

| Model | Verdict self-agreement | Wilson 95% | Mean event-location Jaccard | "Major drift present" agreement | Bar | Result |
|---|---|---|---|---|---|---|
| Opus 5.5 | 13/15 = 0.87 | [0.62, 0.96] | 0.84 | 0.93 | ≥ 0.8 and ≥ 0.6 | pass |
| gpt-6.2-astra | 15/15 = 1.00 | [0.80, 1.00] | 0.91 | 1.00 | ≥ 0.8 and ≥ 0.6 | pass |

- **The sample is small.** With n = 15, one disagreeing session moves agreement by 0.067. The Opus interval is wide: its lower bound, 0.62, does not rule out a true agreement below 0.8.
- **Comparison with v2.** On the same sessions, the earlier v2 pair (Python implementation) gave 1.00 for both models. The Opus drop to 0.87 is within the pre-registered tolerance of 0.15, but it goes the wrong way.
- **Prompt cache.** The second Opus pass ran within the hour on identical inputs and was billed mostly as cache reads. Whether that affects the output was not checked.

### 1.8 Language: Korean and English v2.1 (Opus, 40 public sessions)
- **Setup.** A Korean translation of v2.1 (same procedure, schema and English enum values) judged the same 40 public sessions (20 positive, 20 presumed negative) as the English v2.1 run of §1.6, with Opus 5.5.

| Pair | Verdict agreement | Mean location Jaccard | Four-cause agreement (κ) | Three-group agreement |
|---|---|---|---|---|
| Korean v2.1 vs English v2.1 (language alone) | 0.875 | 0.84 | 0.86 (0.80) | 0.92 |
| v2 vs Korean v2.1 (wording and cause procedure, same language) | 0.80 | 0.80 | 0.74 (0.62) | 0.88 |
| Opus re-running the same v2.1 rubric (§1.7, developer logs) | 0.87 | 0.84 | | |

- **What this supports.**
  - On these sessions, with Opus, the rubric's language changed the verdicts no more than re-running the same rubric does.
  - The shift in cause attribution from v2 to v2.1 (more "agent error", fewer "undeterminable", lower four-cause agreement with the public labels) appears in both v2.1 languages, so it comes from the wording and cause procedure.
- **What it does not cover.**
  - It does not cover astra, a third language, or transcripts written in Korean.
  - Differences between the arms in recall, AUC and label agreement were all below about 0.1, too small to call with 20 positives.

### 1.9 Summaries in the user's language (judged text v2.1-cf92779d+t2)
- **Setup.** The same frozen developer sessions as §1.7, judged once more with the new wording by the Go program: 15 Claude Code sessions with Opus 5.5, and 13 of 15 Codex sessions with gpt-6.2-astra (the run's pre-set budget stopped before the last two started). The new run is compared with the first §1.7 pass; the pass-to-pass agreement of §1.7 is the noise baseline. Pre-registered tolerance: no more than 0.15 below the baseline.

| Judge | Measure | Baseline (old text, two runs) | New vs old | Tolerance | Result |
|---|---|---|---|---|---|
| Opus 5.5 (n = 15) | Verdict agreement | 0.87 [0.62, 0.96] | 0.80 [0.55, 0.93] | ≥ 0.72 | within |
| Opus 5.5 (n = 15) | Mean event-location Jaccard | 0.84 | 0.71 | ≥ 0.69 | within (margin 0.03) |
| gpt-6.2-astra (n = 13) | Verdict agreement | 1.00 [0.80, 1.00] | 0.92 [0.67, 0.99] | ≥ 0.85 | within |
| gpt-6.2-astra (n = 13) | Mean event-location Jaccard | 0.91 | 0.91 | ≥ 0.76 | within |
| both | Quotes not found in the transcript | 1 of 279 | 0 of 146 | ≤ 5% | within |
| both | Schema problems | 0 | 0 | 0 | within |

- **Summary language.** In Korean sessions every summary came out in Korean: 55 of 55 for Opus, 29 of 29 for astra (with the old text all were English). Some sessions that a letter count classified as English, mostly pasted English material, also got Korean summaries. No quote was translated.
- **What this supports.** The new wording did not move verdicts or event locations beyond run-to-run noise on these sessions. Both Opus figures went down, and Opus reported more events (27 against 21–22, all `agent_error`). With 15 sessions this can be neither confirmed nor ruled out as an effect of the wording.

### 1.10 Grouping by meaning (themes across sessions)
After judging, one call per tool groups the judged problems into themes: requests the person had to make in two or more sessions. The report then says, for example, "you had to say this in 3 sessions". A wrong theme would tell a person they kept repeating something they did not. The check therefore asked whether themes are right often enough to show by default.

- **Setup.**
  - Planted themes: six synthetic sets per model (English and Korean; small, medium, heavy), 16 planted themes each, plus distractors that share a topic or are near-misses.
  - Real sessions: the 30 judged developer sessions of §1.7, grouped twice, with each theme rated pair by pair by two blind raters.
  - Pre-registered rule for showing themes by default: false themes at most 5% of kept themes and at least 70% of planted themes recovered (each model and language), and no real theme rated "not the same request" by both raters for most of its pairs.
- **First round (prompt g1).**
  - Planted: 16 of 16 recovered, 0 false themes, for both models and both languages.
  - Real: Codex 5 themes, none failed. Claude Code 2 themes, one failed: it joined four agent mistakes about unrelated subjects under a rule ("do not assert without checking") the person never stated.
  - The planted sets had no distractor of that kind.
- **Changes.**
  - The prompt (g2) now decides themes from the person's own words only.
  - Only the person's quotes and the cited instruction are sent, not the agent's turns.
  - A code check keeps a theme only if the person's stated words occur in at least two of its sessions.
  - A new distractor class was added to the planted sets: three to five unrelated agent mistakes of the same kind.
- **Second round (prompt g2).**
  - Planted: 16 of 16 recovered, 0 false themes, for both models and both languages. The new distractors never became a theme.
  - Real, Claude Code: the wrong theme is gone and the correct one (replies in the wrong language, two sessions) is unchanged. One theme, one rated pair, rated "same" by both raters.
  - Real, Codex: 4 themes, all rated "same" by both raters. One earlier theme is no longer proposed; that is a loss of recall, not a false theme.
  - Both runs gave identical themes.
- **Result.** Both tools pass the rule, so grouping is on by default for Claude Code and Codex. It can be turned off with `--no-grouping` or per tool.
- **What this supports.**
  - The planted sets are clean by design, so they show the step can do the task, not a false-theme rate on real logs.
  - The real sample shows that the specific false theme from the first round does not come back.
  - With one Claude Code theme and four Codex themes, it cannot estimate how often a wrong theme appears on other people's logs.
  - A wrong theme adds one grouping line to the report; the events under it are shown as they are.

## 2. Leak diagnosis (cost leaks)
- **What it measures.** The leak diagnosis is mechanical and reads local logs only. It finds the following and prices each at API list price:
  - cache expiry after long breaks
  - re-writes after mid-session configuration changes
  - repeated commands and consecutive failures
  - re-reading unchanged files
  - oversized tool outputs
- **Check on public traces.** 105,957 trajectories from nvidia/Open-SWE-Traces (CC BY 4.0). The waste ratio built from the computable components **did not separate solved from unsolved tasks** (AUC 0.506–0.522). Simple session size did better (AUC 0.62–0.68).
- **What that means.** The leak report is an accounting of where money went. It is **not** a quality or efficiency score.
- **Coverage.** That check covered only about a third of the waste components. Cache misses and abandoned sessions cannot be computed from those traces.
- **Code check.** Under the research implementation's frozen prices, the Go program reproduces its leak outputs exactly: byte-identical text and zero JSON differences on a 30-day window of the developer's logs, and byte-identical on the synthetic fixtures ([development.md](development.md)). The Go program's default prices differ on purpose: they are the current list prices, which put Codex amounts several times higher.

## 3. Re-validation of v2.1
The English v2.1 rubric changed the judge's input in three ways: language (Korean → English), wording (rewritten, not translated), and the cause procedure. It was re-run against the same pre-registered quality criteria:

1. **Public data, both models, the same 80 sessions** (§1.6). P1, P2 and P4 pass. Over-calling on presumed negatives is 60% (astra) and 36% (Opus). The proxy-label comparison (P5) was not part of this run; it remains a v2 result (§1.2).
2. **Reproducibility (P3)** with the released Go program, 15 sessions per model (§1.7). It passes for both models, with the wide Opus interval stated there.
3. **Language** (§1.8): Korean and English v2.1 agree at the level of run-to-run noise. That result covers Opus on 40 public sessions with mostly English transcripts.
4. **Same-session cross-check on developer logs** (§1.3, §1.4). It was run with rubric v2. It shows that the "agent error" gap comes from the sessions, not the models.
5. **Summaries in the user's language** (§1.9): verdicts and event locations within the tolerance of run-to-run noise, with 0 invented quotes and 0 schema problems.

None of these compares the tool with human labels; the references are model labels audited by humans (§1.1) and agent proxy labels (§1.2).
