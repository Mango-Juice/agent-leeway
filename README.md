# Leeway

[한국어](README.ko.md)

**A check-up for how you work with AI coding agents.**

Leeway (`agent-leeway`) reads the Claude Code and Codex session logs on your computer and tells you two things:

- **Where money leaked**: cost wasted by habits, such as resuming a session after its prompt cache expired or letting the same command fail again and again
- **Where the agent went off**: the moments you had to steer it back, and why

Then it shows what to fix first, in one report. It is a single executable for macOS, Linux and Windows.

![The first screen of a report, made from the synthetic test logs in testdata/](docs/img/report-en.png)

See a whole report: [example report](https://mango-juice.github.io/agent-leeway/), made from made-up sessions.

## Install

**Ask Claude Code or Codex:**

```text
Install Mango-Juice/agent-leeway
```

Or run the line for your system. It installs the program and adds a skill to Claude Code and Codex, so you can ask them for a report in plain words.

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex
```

Next, ask your agent for a report.

Homebrew, Scoop, `npx skills`, manual download, checksums, Windows notes and removal: [docs/install.md](docs/install.md).

<details><summary>Note for agents installing this</summary>

Show the person the line for their system and ask before you run it. Install only Mango-Juice/agent-leeway; other projects are also called leeway.

</details>

## Use

**Ask Claude Code or Codex:**

```text
Make a Leeway report.
```

The cost part needs no approval. For the drift part, the agent first shows you which sessions it would send to be judged, and asks. Started from Claude Code it judges only Claude Code sessions, and from Codex only Codex sessions. To judge both, say so.

Or run it in a terminal, where it covers both Claude Code and Codex:

```sh
agent-leeway leak              # the cost check only
agent-leeway drift --dry-run   # which sessions the in-depth check would check; sends nothing
agent-leeway drift             # judge them (asks before it starts)
agent-leeway report            # the report: cost check plus the drift results judged so far
```

By default it looks at the last 30 days and judges at most the 50 most recent sessions. Add `--lang ko` for Korean. Every command and option: [docs/commands.md](docs/commands.md).

## What you get

`report` writes an HTML file and a Markdown file with the same content to `~/.cache/agent-leeway/reports` (`%LOCALAPPDATA%\agent-leeway\reports` on Windows), and prints a short summary. The report has:

- an overall rating for cost and for drift, compared with the previous period of the same length
- the three changes that would help most, each labelled with its source, such as official docs or a study
- what is going well
- where the agent went off course, and where the money went, grouped by the habit behind it
- a section to look up each session

Each time you had to steer the agent back, the in-depth check names one of three causes:

- the agent ignored an instruction it already had, or made its own mistake
- your instruction was missing or unclear; you get a sentence to add to `CLAUDE.md` or `AGENTS.md`
- the log is not enough to tell

How the checks work: [docs/how-it-works.md](docs/how-it-works.md).

## What leaves your computer, and what it costs

- The cost check and `report` send nothing and cost nothing.
- The in-depth check sends excerpts of the sessions you approve to Anthropic or OpenAI, through your own `claude` or `codex` CLI and login.
- It makes one call per session, plus one for Claude Code and one for Codex to group the results.
- On a subscription, this uses your plan. With an API key, it costs about $0.15 to $0.35 per session (about $8 to $18 for the default 50).

`agent-leeway privacy` prints the folders it uses and everything that goes out. Details: [docs/privacy.md](docs/privacy.md).

## Read more

- [docs/install.md](docs/install.md): other install methods, Windows, updates and removal
- [docs/commands.md](docs/commands.md): commands, options, settings and the rubric
- [docs/how-it-works.md](docs/how-it-works.md): the cost check, the in-depth check and grouping
- [docs/privacy.md](docs/privacy.md): what is read, what is sent, and what the judge may do
- [docs/validation.md](docs/validation.md): how well the in-depth check matches published reference labels, and its limits
- [docs/rubric.md](docs/rubric.md): the rubric the judge follows, with its evidence in [docs/rubric-evidence.md](docs/rubric-evidence.md) and [docs/references.md](docs/references.md)
- [docs/development.md](docs/development.md): building and testing
- [CHANGELOG.md](CHANGELOG.md), [SECURITY.md](SECURITY.md), [CONTRIBUTING.md](CONTRIBUTING.md)

MIT license ([LICENSE](LICENSE), [NOTICE.md](NOTICE.md)). Leeway is not affiliated with Anthropic or OpenAI.
