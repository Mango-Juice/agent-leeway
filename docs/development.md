# Development

Leeway (`agent-leeway`) is a single Go program. This page covers the layout, building, testing, how the outputs were checked against the original research implementation, and a short history.

## Layout

```
cmd/agent-leeway/        main (embeds time zone data)
internal/logs/           log discovery, streaming line reader (any line length, in-process zstd), Claude Code and
                         Codex parsers, per-file cache keyed by size and modification time
internal/pipeline/       sessions and prices, per-response waste, origin (direct vs automation), cache-rewrite table
internal/leak/           leak check, text and JSON output, compare, idle
internal/drift/          rubric, renderer, selection, estimate, CLI preflight, providers, concurrent judging,
                         grouping, summary
internal/proc/           child process trees: process group on Unix, job object on Windows
internal/report/         Markdown and HTML report
internal/install/        skill install and uninstall
internal/cli/            commands
internal/assets/         embedded rubric (v2.1) and SKILL.md template
internal/messages/       all user-facing text
internal/meta/           name, version and every release location
internal/py/             reproductions of the Python behaviors the outputs depend on (see "Parity")
internal/testfake/       stand-in judge used by tests (no model calls)
skill/agent-leeway/      the published skill (a test keeps it equal to the embedded template)
install/                 install.sh (macOS, Linux) and install.ps1 (Windows)
testdata/                synthetic logs, canned judge answers, reference outputs
.github/workflows/ci.yml, release.yml, .goreleaser.yaml
```

**Dependencies outside the Go standard library**

| Module | Used for |
|---|---|
| `github.com/klauspost/compress` (zstd) | Reading Codex's compressed `*.jsonl.zst` logs. Pure Go, no cgo |
| `golang.org/x/text` (unicode/norm) | NFC normalization of project names (macOS paths can be NFD) |
| `golang.org/x/sys` (windows) | Job objects, so a timeout or cancel ends a judge CLI and everything it started |

Licenses are listed in [NOTICE.md](../NOTICE.md).

## Build

```sh
go build -o agent-leeway ./cmd/agent-leeway
./agent-leeway --version
```

- Requires Go 1.27.4 or newer, as pinned in `go.mod`.
- Release builds use `CGO_ENABLED=0` and `-trimpath`, for darwin, linux and windows on amd64 and arm64. All six targets cross-compile from one machine.
- The version comes in at link time: `-ldflags "-X github.com/Mango-Juice/agent-leeway/internal/meta.Version=0.1.0"`. Release builds take it from the tag (GoReleaser drops the leading `v`), so a v0.1.0 build prints `agent-leeway 0.1.0`. A build without the flag prints `0.1.0-dev`.

**Release location.** The repository is `github.com/Mango-Juice/agent-leeway`, which is also the Go module path. The account name is kept in three places:
- `internal/meta/meta.go`
- the install scripts; a test fails if they disagree with `internal/meta`
- `.goreleaser.yaml`

**Releases.** Pushing a `v*` tag runs `.github/workflows/release.yml`: the tests, then GoReleaser builds the six archives and `checksums.txt` into a **draft** GitHub Release, then every archive gets a build provenance attestation. The draft is published by hand after review.

**Environment variables for testing and development**

| Variable | Used by | Effect |
|---|---|---|
| `LEEWAY_RELEASE_BASE` | `install.sh`, `install.ps1` | Download the archive and `checksums.txt` from this URL instead of GitHub Releases. The install tests point it at a local release |
| `LEEWAY_VERSION` | install scripts | Install this tag instead of the latest release |
| `LEEWAY_INSTALL_DIR` | install scripts | Install into this folder |
| `LEEWAY_RELEASE_API` | `self-update --check` | Ask this URL for the latest release instead of the GitHub API |
| `LEEWAY_JUDGE_CMD` | `drift` | Replace the judge CLIs with a local program (the tests' fake judge). `drift` warns when it is set |
| `LEEWAY_CLAUDE_DIR`, `LEEWAY_CODEX_DIR` | all log readers | Read logs from these folders |

## Test

```sh
go vet ./...
go test ./...
```

- **No test calls a model.** Judges are stand-ins (`internal/testfake`), installed as fake `claude` and `codex` executables in a PATH folder whose name contains a space. The tests check that:
  - no prompt text appears on a command line
  - quotes, `% ^ & | < > !`, newlines, Korean and emoji reach the judge unchanged
  - budget, usage-limit, failure, timeout and cancel rules hold with concurrent calls
  - a timeout ends the whole process tree
- **Log reading** is tested on:
  - multi-megabyte lines, CRLF, partial and invalid lines, invalid UTF-8
  - plain and `.zst` twins
  - long and non-ASCII paths
- **The install scripts.** `install.sh` is tested against a local fake release, including a checksum mismatch that must install nothing. `install.ps1` is tested on the Windows runners by `.github/scripts/test-install-ps1.ps1`, under Windows PowerShell 5.1 and PowerShell 7. It installs with `irm … | iex` from a local release and checks the installed program, the license files, a user-Path-only change, a checksum mismatch, the skills, and the documented uninstall.
- **CI.** The CI workflow runs vet and tests on `ubuntu-24.04`, `ubuntu-24.04-arm`, `macos-15`, `macos-15-intel`, `windows-2025` and `windows-11-arm`, runs the `install.ps1` test on both Windows runners, and cross-builds all six targets. Every action is pinned to a commit SHA, and a test checks this.

**Fixtures** (`testdata/`) are fully synthetic. They contain no real session:
- `testdata/logs/`: Claude Code and Codex logs in English, Korean and Spanish, including compressed Codex files.
- `testdata/judge/`: canned judge answers.
- `testdata/expected/`: the reference outputs that the Go outputs must equal, including all intermediate tables, leak text and JSON, compare, idle, rendered drift transcripts, the system prompt, estimates, results, the summary and the report files. These were computed with a fixed clock (2026-01-20T00:00Z) in the time zone Asia/Seoul.

## Parity with the research implementation

The tool was first written in Python during the research phase. The Go program was checked against that implementation, which serves as an *oracle*. The oracle and its fixture generator belong to the private research record and are not part of this repository. What it produced is here as `testdata/expected/`, and `go test ./...` compares against it.

**Results of the parity gates** (full details in the research record):

| Gate | Synthetic fixtures | One developer's real logs (checked locally; numbers not published) |
|---|---|---|
| Reading: every intermediate table, row by row | 13/13 tables equal | 13/13 equal over a 61-day file set |
| Leak: counts exact, amounts within 1e-6 | text and JSON outputs byte-identical | 30-day window: text byte-identical, JSON 0 differences |
| Drift: transcripts byte-identical; cached judgments replayed | all rendered sessions, estimates, records, summary and reports byte-identical | 339 cached judgments: 0 verdict mismatches |
| Build | all six targets build with `CGO_ENABLED=0` | — |

**How the Python behaviors are reproduced.** Python semantics that affect output each have a helper in `internal/py` and a test:
- string length in code points, not bytes
- `json.dumps` and `repr` spellings
- `str.format`
- Python's and numpy's rounding
- Unicode `\s` and `\d`
- `errors='replace'` decoding of invalid UTF-8

**Prices.** The parity tests run with the reference's frozen prices. The program's default prices differ on purpose: they are the current list prices from the providers' price pages (`internal/assets/prices.json`, with source and date per row; `agent-leeway prices`), which put Codex amounts several times higher. Outputs at the default prices are pinned by Go-written expected files in `testdata/expected-go/`.

**Where Go deliberately differs** (under the reference's prices, none changed a number on the fixtures or on the real logs):
- Codex `archived_sessions` are read (turn off with `--no-archived`).
- Plain/`.zst` twins are matched case-insensitively on Windows.
- Broken or unknown log lines are skipped and **counted**, not silently dropped.
- Local time follows daylight saving.
- Symbolic-link loops are guarded.
- Judge calls run concurrently, with a per-call timeout and whole-tree cancellation.
- Windows is supported.

**When behavior changes on purpose**, the expected outputs in `testdata/expected/` must be regenerated from the Go program, or the affected comparisons retired. The Python oracle is not maintained further.

**Real CLIs.** Judge calls are tested against stand-ins built from the same output formats. One fixed probe call per real CLI, with no session content, confirmed that `claude` and `codex` accept the lock-down flags and that neither used a tool (`TestProbeRealCLIs`, run only on request). The v2.1 reproducibility run then made 60 real judge calls with this program (claude 2.1.329, codex-cli 0.171.0). Every call accepted the flags, stayed one turn with no tool use, and parsed with 0 schema problems ([validation.md](validation.md) §1.7).

## How the project came about (short history)

The project began as a research question: **can "how efficiently someone uses an AI coding agent" be defined as a number?** The research record is kept private because it was built on one developer's real session logs.

1. **Efficiency scores did not hold up.**
   - Four candidate metrics were defined on local Claude Code and Codex logs: cost per outcome, waste ratio, correction ratio and code survival.
   - The waste ratio tracked the developer's own session ratings only weakly.
   - On public traces it did not separate solved from unsolved tasks.
   - So the project dropped scores and rankings.
2. **What survived is a diagnosis, not a score.**
   - The **leak check** shows where money or plan quota leaked, grouped by the habit that caused it, with a change for each habit.
   - The **drift check** finds sessions where the agent left the user's intent, and whether the cause was the agent, missing or ambiguous instructions, or undeterminable.
3. **The drift rubric was built from disagreements, then checked against the literature.**
   - v0/v1 rules came from cases where judgments diverged.
   - An agent labeler applied v1 to 120 sessions, and its sticking points produced 17 proposed revisions.
   - A literature review graded every rule ([rubric-evidence.md](rubric-evidence.md), [references.md](references.md)).
   - v2 merged these. v2.1 is the English canonical text, with a fixed cause procedure.
4. **Validation was pre-registered** and run on public data and on the developer's logs ([validation.md](validation.md)). v1 and v2 met the pre-registered bars. v2.1 passed P1, P2 and P4 on public data and P3 with this Go program on 15 developer sessions per model.
5. **The Python prototype was ported to a single Go executable**, so users install nothing else. The port was checked for exact parity under the reference's prices, as described above.
6. **Running the judge through the user's own official CLI was a deliberate choice.** The tool never handles credentials and never calls a model provider with its own key or the user's subscription token.
