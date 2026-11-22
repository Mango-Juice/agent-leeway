# Evidence review for the drift rubric

> This review was written for **rubric v1** (2026-11-21). Most of its revision proposals were adopted in v2 and carried into v2.1 ([rubric.md](rubric.md)). The rule names below follow v1. Where v2/v2.1 changed a rule, the current rubric's Part C gives the final grade.

Rubric v1 was built from the maintainer's own judgment. A public default needs outside support for each rule, so every rule was checked against the literature. Sources and how much of each was read (full text, partial, abstract, snippet) are listed in [references.md](references.md). This was a literature review only; no judging experiments were run for it.

**How to read it.** "(summary)" next to a figure means it came from an automated summary of the source, not from the original text. Everything else was checked in the original.

**Evidence strength**
- **strong**: two or more large real-world studies use the same definition or distinction.
- **moderate**: one study, or indirect support from several.
- **weak**: guidelines, incidents or small studies only.
- **none**: nothing found; the rule is a design choice.

## 0. Summary

1. **Defining a drift event as "the user had to correct the agent" (R2) fits the literature best.** The two largest studies both define misalignment through user pushback: Tang26 with 20,574 sessions and Baumann26 with about 18,000. Both also count **failure reports** ("it doesn't work"), which v1 left out.
2. **Silent drift (R3) is the area the literature deliberately leaves out.** Tang26 treats silent rejection and direct edits as latent misalignment and puts them out of scope.
   - Hand edits and re-prompts can be measured, but their validity is low. About 50% of same-region re-prompts were a different task (Nam25b).
   - Recommendation: split R3. Request-type mismatches that the record shows directly stay events. Behavioral signals become weak signals.
3. **The violation / missing-instruction split covers only part of the causes in the literature.** Tang26 has seven causes:
   - underspecified instruction 15.4%
   - instruction-following failure 36.5%
   - cannot determine 26.9%
   - scope overreach 9.5%
   - premature action 11.1%
   - context loss 4.3%
   - default override 2.4%

   Premature action and wrong diagnosis fit neither of v1's two causes. Recommendation: a third cause, **agent error**. Cause is also the axis on which judges agree least (human–human 0.65, judge accuracy 0.72). An undeterminable share above one in four is normal and should be reported.
4. **"Add a sentence to your instructions" helps only partly.**
   - Writing all requirements down did not reliably help. With 19 requirements stated, accuracy fell to 79.7–85.0% (Yang25).
   - Models rarely ask by default (Vijay26).

   Missing-instruction events should therefore also say whether the agent should have asked.
5. **The three "skipped human step" cases (R6) are strongly supported.** Company guidelines (OpenAI-MS, Anthropic25), field studies (Tang26 S3, S7) and benchmarks (Smyth26) all cover them. Base rates of false completion vary widely:
   - 22.6% of real-world misalignment episodes
   - 52.8% of incomplete review runs explicitly overclaimed
   - 1.8% of SWE-chat sessions with severe overselling

   The "provisional" mark can go.
6. **Size (R7) is measured on a different axis in the literature.** v1 measures lost work. The literature grades harm and reversibility (Tang26 damage levels; AlHasan26, a CVSS-style 5-point scale). Recommendation: keep both axes, each coarse. Judges are least accurate on severity (0.64).
7. **"The amount of revision follows the task's importance" (v1 premise) has no support.** No study measured importance against revision counts. On the contrary, repeating or rephrasing a request was most strongly tied to deficient responses (Michel26, abstract). Recommendation: soften it to "counts alone do not decide".

## 1. Evidence by rule

### Premise: tokens, time, turns and revision counts are not evidence; revision volume follows the task's importance

| Item | Content |
|---|---|
| Supporting | Tie26 (abstract): with 21 students and 5 professionals building a web app with ChatGPT, each extra prompt lowered the probability of abandoning by 17%. Long exchanges are not in themselves a sign of failure. Nam25b: of about 130 same-region re-prompt pairs, about 50% were a different task, so re-prompt counts are not failure counts. Ziegler22: among 2,047 respondents, acceptance rate correlated with perceived productivity (ρ = 0.24); all persistence (unchanged-code) measures correlated less. |
| Against / different | DiCuffa25: "about 4 interactions on average to a satisfactory outcome", and used fewer than 5 prompts as an efficiency threshold (iteration count as a quality measure). Michel26 (abstract): repetition and rephrasing were most strongly tied to negative feedback on deficient responses. Nam25b reads re-prompting as a sign of struggle. |
| Base rates | Re-prompts: about 7% of about 100,000 requests (same prompt within 10 minutes, same region within 3 lines). Re-requests after rejection 24.3%, after acceptance 11.9% (Nam25b). |
| Strength | "Counts alone do not decide": **moderate**. "Follows importance": **none**. |
| Recommendation | **Revise** to "counts alone do not decide; repeating the same instruction is an R2 event". |

### R1: judge requests together; one correction spread over several requests is one event

| Item | Content |
|---|---|
| Supporting | Tang26 reads each whole session at once and extracts one record per breakdown, not per request. Michel26 (abstract): a "substantial minority" of repairs target earlier context. Cemri25 FM-1.4 (loss of conversation history) and Tang26 C4 (context loss, 4.3%) make failing to carry earlier requests a failure type in itself. |
| Against / different | Baumann26 classifies pushback per prompt, so one correction can be counted several times. |
| Base rates | Tang26 kept 16,118 of 29,896 extracted episodes (53.9%) after validation: over-extraction, such as counting one thing several times, is common. |
| Strength | **moderate** |
| Recommendation | **Keep.** Note in reports that turn-level rates (SWE-chat about 46%) are not comparable with event counts. |

### R2: verbal correction is an event (revert, delete, repeated instruction, change of method)

| Item | Content |
|---|---|
| Supporting | Tang26 defines misalignment as "observable breakdowns … that surface through developer correction or pushback in conversational logs". Baumann26 defines pushback as a prompt where the user resists, corrects, redirects or takes over, with subtypes correction, rejection and failure report. Michel26 (abstract): user repair (rephrasing, correcting, clarifying, asking for more information) signals breakdown. |
| Against / different | Michel26 (abstract): repair is "a weak standalone signal of failure", common after non-deficient responses too. Requests for more information were more common after non-deficient responses, consistent with not counting questions (R5). Baumann26 does not separate refinement, so its scope is wider. |
| Missing from v1 | **Failure reports** ("it errors", "it doesn't run"): a pushback subtype in Baumann26. Tang26 S7's examples ("the next turn reveals otherwise") are the same situation. **Interruptions**: 4.5% in Baumann26. |
| Base rates | Pushback in about 46% of Claude Code prompts, 50.2% with interruptions (Baumann26). 16,118 misalignment episodes in 20,574 sessions (Tang26). |
| Strength | **strong** |
| Recommendation | **Revise**: consider failure reports and interruptions. v2 kept failure reports out as *defect reports* (maintainer decision) and counts interruptions only when the direction then changes. |

### R3: silent drift is an event

R3 mixed two different kinds of signal, so they were assessed separately.

**R3a. Request-type mismatch** (answering a question by acting; executing when a plan was asked for)

| Item | Content |
|---|---|
| Supporting | Tang26 S2 "misread developer intent" (26.95%) and S4 "self-initiated overreach" (10.20%: actions beyond the stated scope). Anthropic25 describes reviewing and approving the whole plan before execution, and says Claude Code is read-only by default and asks before modifying code or systems. OpenAI-MS "Act within an agreed-upon scope of autonomy": keep to the agreed scope; if the task cannot be done within it, notify the user and seek approval. Cemri25 FM-2.3 "task derailment" (7.4%). |
| Against / different | Tang26 counts these only when the developer pushed back. |
| Strength | **moderate** (the definition is well supported; counting it without pushback is not) |
| Recommendation | **Keep.** It can be judged from the request text and the action alone. |

**R3b. Behavioral signals** (discarding output and asking again elsewhere; editing by hand)

| Item | Content |
|---|---|
| Supporting | Baumann26 attributes code to human or agent through commits. Across modes, human deletion removed 29.8% of agent code and agent self-overwrites 8.1%. Self-overwrites "typically occur when the user pushes back and instructs the agent to reimplement". Tie26 (abstract): unhelpful responses made abandonment 11 times more likely, and 17 participants eventually abandoned ChatGPT. |
| Against / different | Tang26 puts "silently rejecting output or editing code directly" **out of scope** as latent misalignment. Developer takeover is 5.52% of resolved episodes (summary), which the authors call an underestimate. Baumann26 cannot tell whether higher survival in vibe coding reflects better output or less scrutiny, and human deletion can be ordinary cleanup. Nam25b: about half of same-region re-prompts were a different task. Ziegler22: persistence measures correlated less with perceived productivity than acceptance did. Abandoned sessions leave no commits and are invisible (Baumann26 sensitivity footnote). |
| Base rates | Coding efficiency 59.4%, survival 65.4% (Baumann26 Table 3, excluding human-only sessions; N not found in the text). |
| Strength | Measuring the behavior: **moderate**. Reading it as drift: **weak**. |
| Recommendation | **Revise**: show as weak signals, count as an event only with other evidence (a complaint just before discarding, the same request repeated in another session), and make promotion a setting. Restarts across sessions are invisible within one session's log. |

### R4: pasted critiques from other agents or services default to refinement

| Item | Content |
|---|---|
| Supporting | No direct evidence. Indirect: Michel26 (abstract), requests for more information are more common after non-deficient responses. Shankar24 (abstract), "criteria drift": people define their criteria while grading outputs, so a raised bar from outside critique can be criteria forming rather than a defect. |
| Against | None found. |
| Strength | **none** (design choice) |
| Recommendation | **Keep, with a setting**: state that it is a design choice and let users choose by critique source (another model, a review bot, CI). |

### R5: not events (approval, next step, question, raised bar after completion, change of mind, complaining question only)

| Item | Content |
|---|---|
| Supporting | Baumann26's classifier has a separate non-pushback class. Michel26 (abstract): repair is common after non-deficient responses. Shankar24: criteria change while viewing outputs, which supports treating post-completion raised bars as non-events. |
| Against / different | **Change of mind**: Tang26 C1 "underspecified instruction" includes instructions that are "ambiguous, underspecified, or **inconsistent**", so user-side changes still count as episodes, attributed to the user side. **Complaining questions**: in Nam25a the sentiment of the next user turn signals the quality of the previous AI turn. Of about 36,000 turns, about 7% were negative; automated labels matched humans in 75% of cases (κ 0.72). Baumann26's pushback definition includes "resists". |
| Strength | Approvals, next steps and questions excluded: **moderate**. Change of mind and complaining questions excluded: **weak** (the literature counts them) |
| Recommendation | **Partly revise**: keep change of mind as a non-event but list it as "intent change". Keep complaining questions as non-events by default, with a setting to flag negative sentiment. |

### R6: skipping a human's step (only three cases)

| Case | Supporting | Base rates |
|---|---|---|
| (a) Skipping something the user explicitly reserved ("I'll do it", "I'll decide", "ask me first") | Tang26 S3 "developer constraint violation", the most frequent symptom. Tang26 C5 "default-driven override" (defaults such as best practice override an explicit constraint, 2.44%). Anthropic-C: ask for clarification in cases of genuine ambiguity. Cemri25 FM-1.1 "disobey task specification" (11.8%). | S3 38.33% (CLI 49.49%, IDE 32.26%); 73.68% of S3 episodes attributed to C6 instruction-following failure (Tang26) |
| (b) Hard-to-reverse or outward actions without confirmation | Anthropic25: humans should retain control before high-stakes decisions; ask before modifying code or systems. OpenAI-MS: tool actions that are "difficult or impossible to reverse" call for "extra care", and side effects are treated as real unless stated otherwise. AlHasan26 "destructive operations": irreversible commands that delete user data, code or system files without authorization. Replit25 (snippet): production database deleted during a code freeze. | 134 destructive operations (24.5%) and 100 authorization bypasses (18.3%) among 547 confirmed incidents (AlHasan26, summary). Hard-to-reverse damage in 0.07% of real-world episodes, 11 cases (Tang26, summary) |
| (c) Reporting checks that were not done, or filling steps with placeholders and calling it done | Tang26 S7 "inaccurate self-reporting": misreports the status of its own work. Smyth26: overclaiming = a final response asserting actions or completion contradicted by its own transcript. AlHasan26 "deception" (unsupported completion claim) and "fabrication" (plus forged evidence). Cemri25 FM-3.2 no or incomplete verification (8.2%), FM-3.3 incorrect verification (9.1%). Anthropic-C (full text): if no general fix is found, tell the user rather than special-casing tests. | S7 22.58% (Tang26). Of 774 incomplete review runs: explicit completion claims 52.8%, gap omitted 27.5%, honest partial report 19.6% (Smyth26; 12 models, 1,140 runs). Severe overselling in 1.8% of SWE-chat sessions (n = 4,788) (Transluce26). Overclaiming runs missed 58.2% of planted defects, runs that touched every file 32.4% (Smyth26) |

| Item | Content |
|---|---|
| Against / different | Tang26 S4 "self-initiated overreach" (10.20%) and Trong26's "unrequested file creation, update or deletion" count even reversible actions. v1 says routine reversible choices are not events, so it is narrower. If the user objects, R2 catches it anyway, so the real difference is overreach without pushback. Tang26 reports 27.56% overlap between S7 and S3, so (c) and (a) can co-occur. |
| Trend | Tang26 (abstract): overall rates decline, but constraint violations and inaccurate self-reporting grow in share. |
| Strength | (a), (b), (c): **strong** |
| Recommendation | **Keep and drop "provisional".** Make the action list in (b) a user setting. Precedents: MagenticUI's always/maybe/never-irreversible tags (snippet) and Anthropic25's per-action allow/approve/block permissions. Make overreach without pushback an option. |

### Causes (instruction violation / missing or ambiguous instruction / undeterminable)

| Item | Content |
|---|---|
| Supporting | Tang26 separates C1 "underspecified instruction" (15.36%) from C6 "instruction-following failure" (36.49%), so the distinction itself is supported. 44.10% of S2 "misread intent" episodes co-occur with C1. Yang25: underspecification is "the omission of essential requirements in a prompt, such that multiple valid but inconsistent behaviors remain possible". Underspecified prompts were about twice as likely to regress across model changes: 5.9% of requirements dropped more than 20% when unspecified. So "write it down" has support. Cemri25 also separates FM-1.1 (disobey specification) from FM-2.2 (fail to ask for clarification, 6.8%). |
| Against / different | (1) **Two causes do not cover everything.** Tang26's C2 scope overreach (9.47%) and C3 premature action (11.11%, acting before gathering enough project state) are neither violation nor omission. Symptoms S1 wrong project diagnosis (11.56%) and S5 faulty implementation (17.82%) may have causes unrelated to instructions. (2) **Writing it down does not solve it.** Yang25: with one requirement specified, accuracy averaged 98.7%. With 19 specified at once it fell to 85.0% (gpt-4o) and 79.7% (Llama-3.3-70B), and 37.5% of requirements dropped by more than 5%. Models also met unspecified requirements at ≥98% accuracy in 41.1% of cases. (3) **The agent may be the one who should have asked.** Vijay26: without explicit encouragement models almost never ask, even for severely underspecified input. Asking recovers up to 74% over the non-interactive setting. Models told underspecified from well-specified inputs with 0.47–0.89 accuracy. Anthropic-C: ask for clarification in cases of genuine ambiguity. |
| Base rates | Cannot determine 26.85% (Tang26 C7). On the cause axis, human–human agreement was 0.65 and judge accuracy 0.72 [95% CI .63–.80], the lowest of six axes (Tang26 Table 2). |
| Strength | Separating violation from omission: **strong**. That two causes are enough: **contradicted**. The "user can fix it" remedy: **moderate** |
| Recommendation | **Revise**: add a fourth cause, *agent error*; attach "should the agent have asked first?" to missing-instruction events; always show the undeterminable share. |

### R7: size (major / minor)

| Item | Content |
|---|---|
| Supporting | Tang26: 90.50% of episodes impose effort and trust costs, not system damage, so for most drift only the lost-work axis matters, as in v1. 25.00% of Tang26 S5 faulty implementations reached code damage (DS2 or worse). |
| Against / different | The literature grades harm and reversibility. Tang26 (summary): DS0 none 0.08%, DS1 effort and trust only 90.50%, DS2 easily reversed damage 8.44%, DS3 hard-to-reverse damage 0.07% (11 cases), DS4 unobservable 0.91%. AlHasan26 (full text), a CVSS-inspired 5-point scale: 5 critical (immediate, irreversible harm), 4 high (severe but recoverable, extensive manual intervention), 3 medium (silent degradation), 2 low (maintainability problems), 1 negligible. 326 of 547 incidents were high or critical. MIT-AIR: five levels from negligible to catastrophic for each of 13 harm categories. |
| Reliability | Damage severity: human–human 0.71, judge accuracy 0.64 [.54–.73], the lowest judge accuracy of Tang26's six axes. |
| Strength | Having size levels: **moderate**. Using lost work as the criterion: **weak** (no precedent on that axis) |
| Recommendation | **Revise**: two axes, lost work (major/minor) and harm (none / easy to reverse / hard to reverse). v2 reduced harm to a single yes/no. |

### Session verdicts

| Item | Content |
|---|---|
| Supporting | No direct precedent for session-level labels. Indirect: in Tang26, misalignment in one session made misalignment in the next session of the same repository more likely (0.519 vs 0.336), which supports grouping by session or project. |
| Against / different | Human agreement on Baumann26's session success score was ICC 0.503 (summary). Several usage-reporting tools deliberately avoid a single score. |
| Gaps in v1 | No verdict for major events with mixed causes, or with undeterminable causes. |
| Strength | **none** (design choice) |
| Recommendation | **Revise**: add precedence rules; give the worst verdict a neutral public name; make names configurable. |

### Report and mandatory quotes

| Item | Content |
|---|---|
| Supporting | Tang26 grounded every episode in "direct quotations with turn numbers". All 200 sampled quotes were found verbatim in the session, and extraction precision was 0.93 [.89–.96]. Wang23: making the judge write evidence first (MEC) raised GPT-4's agreement with humans from 52.7% to 58.7% accuracy (κ 0.24 → 0.30, n = 80). Adding order balancing reached 62.5% (κ 0.37), still below average human agreement (71.7%, κ 0.54). |
| Limits | Requiring quotes may lower recall. Tang26's recall check on 30 sessions averaged 1.77 on a 0–2 scale. |
| Strength | **moderate** |
| Recommendation | **Keep and strengthen**: check quotes verbatim in code and drop events whose quotes are missing (as Tang26 did). Keep "no scores, no rankings". |

## 2. Measurement: what the literature recommends for LLM judges

| Source | Finding | Implication |
|---|---|---|
| Zheng23 | GPT-4–human agreement 85% without ties (859 votes) and 66% with ties (1,343); humans with each other 81% and 63%. With the order of two similar answers swapped, GPT-4 stayed consistent in 65.0% of cases, or 77.5% with few-shot examples. | Agreement swings by about 20 points depending on how ties are handled. Always report how it was computed. |
| Wang23 | Swapping the order flipped GPT-4's verdict in 46.3% of comparisons between two similar models. Evidence first: +6 points accuracy. | Quotes help but do not reach human level. Shuffle presentation order. |
| Thakur25 | Percent agreement barely separates judges; Scott's π is more informative. High agreement can hide very different scores. Most judges are lenient. | Report chance-corrected agreement (κ or π) alongside percent agreement. Tang26 reported percent agreement only because severe class imbalance deflates κ. Report both. |
| Tang26 | Human agreement and judge accuracy reported per axis: cause 0.72, severity 0.64, resolution 0.96. | Validate each axis separately. Mark cause and size as lower-reliability axes in reports. |
| Cemri25 | Human κ 0.88 (three annotators). o1 judge: accuracy 0.94 and κ 0.77 with few-shot examples, κ 0.58 without. | Human-labelled examples in the judge prompt raise agreement substantially. |
| Lee25 | Splitting a judgment into yes/no checklist questions greatly raised agreement between judge models (SummEval κ: G-Eval 0.19, CheckEval 0.48). | Phrasing rules as yes/no questions ("did the user ask to undo it?") helps. |
| Smyth26 (summary) | Re-judging the same run eight times: 91.1% unanimous; a single verdict matched the majority 97.7% of the time. | Re-judging stability is a cheap lower bound, not a substitute for human agreement. |
| Shankar24 (abstract) | Criteria drift: criteria get defined while grading outputs. | Rubrics change as they are applied. Version them and re-validate on a human sample after each change. |

**Recommended validation before release**
1. Keep a human-labelled sample of at least 100 items per axis (Tang26 scale). At least two people other than the developer label independently, and a gold set is built by agreement.
2. Report judge accuracy with 95% intervals for each axis (event, cause, size, session verdict), with both percent agreement and κ (or π).
3. Check quotes verbatim in code. Measure precision on a sample and recall by re-reading at least 30 sessions.
4. Measure order-shuffle and eight-fold re-judging stability.
5. Validate externally on public data: the Tang26 labels (MIT; cause and symptom axes) and the Baumann26 pushback annotations (gated).

## 3. Items the literature suggests that v1 lacked

| Item | Source | Proposal (and what v2/v2.1 did) |
|---|---|---|
| Failure reports | Baumann26 pushback subtype; Tang26 S7 examples | v2: separate defect-report tally; false completion stays an event |
| User interruptions | Baumann26 (4.5%) | v2: counted only when the direction then changes |
| Symptom tags | Tang26's seven symptoms | Optional; useful for comparing with external base rates (not yet adopted) |
| Agent judgment or execution error as a cause | Tang26 C2, C3; Cemri25 | v2: *agent error* |
| Failure to ask for clarification | Cemri25 FM-2.2 (6.8%), Vijay26, Anthropic-C | v2: `should_have_asked` on missing-instruction events |
| Resolution | Tang26: resolution visible in only 9.33%, 91.49% of those after pushback | v2: resolution field |
| Harm axis | Tang26 DS levels, AlHasan26 | v2: irreversible harm (yes/no) makes an event major |
| Overreach without pushback | Tang26 S4, Trong26 | v2: setting S7 |
| Intent change | Tang26 C1 includes inconsistent instructions | v2: listed separately, not an event |

## 4. Limits of this review
- The core sources (Tang26, Baumann26, Smyth26) are 2026 preprints, not yet peer-reviewed. Tang26 and Baumann26 also used LLM judges, so the external base rates themselves contain judge error.
- Several sources were read only in their first part. Tang26's damage-level details, SWE-chat's human–LLM agreement figures and AlHasan26's per-category counts came only from summaries and are marked "(summary)".
- The classic conversation-repair paper (Schegloff77) and the Replit incident were seen only in search snippets and were not used to grade evidence.
- No study was found that directly measures the relation between revision counts and task importance or complexity. Two candidates could not be read.
