# References

Checked on 2026-11-21. The rubric ([rubric.md](rubric.md)) and the evidence review ([rubric-evidence.md](rubric-evidence.md)) cite these works by the bracketed keys.

**What was read**
- **full text**: the passage containing the cited figure or definition was read in the original.
- **partial**: the first part was read in the original; later parts only through an automated summary. Figures that came only from the summary are marked "(summary)" where they are used.
- **abstract**: only the abstract or the bibliographic page.
- **snippet**: only search-result summaries. Never used as the sole basis for an evidence grade.

**Status**
- **peer-reviewed**: publication confirmed.
- **peer-reviewed (unverified)**: the venue appears only in the page header or in search results.
- **preprint**
- **non-academic**: blog, guideline or news.

## A. Taxonomies of failure and misalignment in human–agent coding sessions

| Key | Work | URL | Status | Read |
|---|---|---|---|---|
| Tang26 | Tang, Chen, Xu, Shi, Huang, McMillan, Dong, Li. "How Coding Agents Fail Their Users: A Large-Scale Analysis of Developer-Agent Misalignment in 20,574 Real-World Sessions." arXiv 2605.29442v2, 2026-08-31 | https://arxiv.org/html/2605.29442 | preprint | partial. Definitions, symptom and cause tables, and judge validation in full; damage-level and resolution details from the summary |
| Baumann26 | Baumann, Padmakumar, Li, Yang, Yang, Koyejo. "SWE-chat." arXiv 2604.20779v2, 2026-10-01 | https://arxiv.org/html/2604.20779v2 | preprint | partial. Pushback definition, rates and the code-survival table in full; human–LLM agreement figures from the summary |
| Cemri25 | Cemri, Pan, Yang, Agrawal et al. "Why Do Multi-Agent LLM Systems Fail?" arXiv 2503.13657v3, 2025-10-26 | https://arxiv.org/html/2503.13657v3 | peer-reviewed (unverified: NeurIPS 2025 poster page in search results) | partial. The 14 failure modes, frequencies and agreement in full |
| Smyth26 | Smyth, Mantilla-Ramos, Tikeng Notsawo, Helbling, Tosato, Merzouk, Dziri, Gidel, Tosato. "Quantifying Overclaiming Propensity in Frontier LLM Agents." arXiv 2609.20812v3, 2026-09-22 | https://arxiv.org/html/2609.20812 | preprint | partial. Definition, Table 1 rates and the missed-defect comparison in full; judge model and re-judging stability from the summary |
| Transluce26 | Transluce. "Measuring coding agent misalignment in the wild." Cross-posted on LessWrong, 2026-08-05 | https://www.lesswrong.com/posts/smE9h9RnaK7FWKBZ2/measuring-coding-agent-misalignment-in-the-wild-1 | non-academic (research blog) | partial (first 40k characters) |
| AlHasan26 | Al Hasan, Biswas. "What Breaks When LLMs Code? Characterizing Operational Safety Failures of Agentic Code Assistants." arXiv 2605.30777v3 | https://arxiv.org/html/2605.30777 | peer-reviewed (unverified: header says accepted at ASE '26) | partial. Sample, definitions and severity scale in full; per-category counts from the summary |
| Trong26 | Do Xuan Trong et al. "SWE-MISBEHAVIOR: Benchmarking and Mitigating Boundary Violations in Software Coding Agents." arXiv 2610.18206v1 | https://arxiv.org/html/2610.18206 | preprint | partial. Metric definitions in full; per-model rates from the summary |
| Tie26 | Tie et al. "'Should I Give Up Now?' Investigating LLM Pitfalls in Software Engineering." arXiv 2411.09916v2, 2026-03-10 | https://arxiv.org/html/2411.09916v2 | preprint | Abstract figures in full; method and the definition of abandonment from the summary |

## B. Underspecification, ambiguity and clarification

| Key | Work | URL | Status | Read |
|---|---|---|---|---|
| Yang25 | Yang, Shi, Ma, Liu, Kästner, Wu. "What Prompts Don't Say: Understanding and Managing Underspecification in LLM Prompts." arXiv 2505.13360v2, 2025-10-07 | https://arxiv.org/html/2505.13360v2 | peer-reviewed (unverified: ACL Findings 2026 in search results, https://aclanthology.org/2026.findings-acl.441/) | partial (§3.1–3.4 in full) |
| Vijay26 | Vijayvargiya, Zhou, Yerukola, Sap, Neubig. "Ambig-SWE: Interactive Agents to Overcome Underspecificity in Software Engineering." arXiv 2502.13069v3, 2026-02-21 | https://arxiv.org/html/2502.13069v3 | peer-reviewed (unverified: ICLR 2026 poster page in search results) | partial (through §4.3 in full) |

## C. Implicit signals, conversational repair and iteration

| Key | Work | URL | Status | Read |
|---|---|---|---|---|
| Nam25a | Nam, Salawa, Chandra (Google). "Reading Between the Lines: Scalable User Feedback via Implicit Sentiment in Developer Prompts." arXiv 2509.18361v1, 2025-09-22 | https://arxiv.org/html/2509.18361v1 | preprint | full text |
| Nam25b | Nam et al. (Google). Re-prompting analysis of an IDE code-transformation feature. arXiv 2504.20196v2, 2025-12-19 | https://arxiv.org/html/2504.20196v2 | peer-reviewed (unverified: header says ICSE 2026) | partial (first 40k characters) |
| Ziegler22 | Ziegler et al. (GitHub). "Productivity Assessment of Neural Code Completion." MAPS '22 | https://arxiv.org/abs/2205.06537 | peer-reviewed (workshop) | partial (definitions and main result in full) |
| Michel26 | Michel et al. "Reading Between the Turns: User Repair as a Context-Dependent Signal of Breakdown and Recovery in Multi-Turn LLM Assistants." LUHME 2026, pp. 38–53 | https://aclanthology.org/2026.luhme-1.3/ | peer-reviewed (workshop) | abstract |
| Schegloff77 | Schegloff, Jefferson, Sacks. "The Preference for Self-Correction in the Organization of Repair in Conversation." Language 53(2):361–382, 1977 | https://muse.jhu.edu/article/453740/summary | peer-reviewed | snippet (the page blocked automated access) |
| DiCuffa25 | DiCuffa et al. "Exploring Prompt Patterns in AI-Assisted Code Generation." arXiv 2506.01604v1 | https://arxiv.org/html/2506.01604v1 | preprint | partial |

## D. Agent behavior guidelines, incidents and severity

| Key | Work | URL | Status | Read |
|---|---|---|---|---|
| OpenAI-MS | OpenAI. Model Spec, 2026-10-29 version | https://model-spec.openai.com/2026-10-29.html | non-academic (company guideline) | partial. Agreed scope, side effects and irreversibility in full; the "Do not lie" and clarifying-question sections not read |
| Anthropic25 | Anthropic. "Our framework for developing safe and trustworthy agents." 2025-08-04 | https://www.anthropic.com/news/our-framework-for-developing-safe-and-trustworthy-agents | non-academic (company guideline) | full text |
| Anthropic-C | Anthropic. Claude's constitution (no date shown) | https://www.anthropic.com/constitution | non-academic (company guideline) | partial (first 40k characters in full; passages on irreversible actions and honesty only from the summary) |
| Replit25 | Fortune, "AI coding tool Replit wiped database...", 2025-07-23; AI Incident Database, Incident 1152 | https://fortune.com/2025/07/23/ai-coding-tool-replit-wiped-database-called-it-a-catastrophic-failure/ , https://incidentdatabase.ai/entities/replit-ai-agent/ | non-academic (news, incident database) | snippet |
| MIT-AIR | MIT AI Risk Repository, AI Incident Tracker harm taxonomy | https://airisk.mit.edu/ai-incident-tracker/harm-taxonomy | non-academic | full text (only the ends of the 5-level scale are named; levels not defined) |
| OpenAI-CI | OpenAI. "Classifying AI control incidents" (v0.1 draft) | https://openai.com/index/classifying-ai-control-incidents/ | non-academic | snippet (fetch blocked) |
| MagenticUI | Microsoft Research. "Magentic-UI: Towards Human-in-the-loop Agentic Systems." arXiv 2507.22358 | https://arxiv.org/pdf/2507.22358 | preprint | snippet |

## E. Validity of LLM judges

| Key | Work | URL | Status | Read |
|---|---|---|---|---|
| Zheng23 | Zheng et al. "Judging LLM-as-a-Judge with MT-Bench and Chatbot Arena." NeurIPS 2023 Datasets and Benchmarks | https://arxiv.org/html/2306.05685v4 | peer-reviewed | partial (§2–4 and Tables 2–7 in full) |
| Wang23 | Wang et al. "Large Language Models are not Fair Evaluators." arXiv 2305.17926v2 | https://arxiv.org/html/2305.17926v2 | peer-reviewed (unverified: ACL 2024 in search results) | partial (§2–5 in full) |
| Thakur25 | Thakur et al. "Judging the Judges." GEM2 2025 (ACL Anthology pp. 404–430) | https://arxiv.org/html/2406.12624v6 | peer-reviewed (workshop) | partial (§1–4.1 in full) |
| Lee25 | Lee et al. "CheckEval." arXiv 2403.18771v3 (EMNLP 2025 according to the arXiv comments) | https://arxiv.org/html/2403.18771v3 | peer-reviewed (unverified) | partial |
| Shankar24 | Shankar, Zamfirescu-Pereira, Hartmann, Parameswaran, Arawjo. "Who Validates the Validators?" arXiv 2404.12272 (UIST '24 in search results) | https://arxiv.org/abs/2404.12272 | peer-reviewed (unverified) | abstract |

## Found but not used as evidence
- Champa et al., MSR 2024 (prompts per task type in DevGPT): the PDF link redirected and the text was not read.
- Wang et al., "Using LLMs in Software Design," arXiv 2605.01392: PDF text extraction failed. According to search snippets it reports a median of 5 rounds per task and suggests that iteration grows with design complexity. Not verified.
- Chatbot repair studies (Galbraith 2024, Braggaar et al. 2023 and others): search snippets only.
