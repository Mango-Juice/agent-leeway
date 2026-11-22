# Contributing to Leeway

Thank you for helping. This page covers building, testing, and the rules that keep the tool's measurements comparable over time.

**Never commit real logs.** Real session logs, transcripts, reports and instruction files can contain code, prompts and secrets. Everything under `testdata/` is synthetic.

## Build and test

```sh
go build -o agent-leeway ./cmd/agent-leeway
go vet ./...
go test ./...
```

- **Go version.** The version is pinned in `go.mod`. Release builds use `CGO_ENABLED=0` and cover darwin, linux and windows on amd64 and arm64.
- **No test calls a model.** Judges are stand-ins (`internal/testfake`) put on PATH as fake `claude` and `codex` executables.
- **The install scripts are tested too.** `install.sh` runs in the Go tests. `install.ps1` runs on the Windows CI runners (`.github/scripts/test-install-ps1.ps1`).
- **CI.** The workflow runs vet and the tests on Linux, macOS and Windows (x64 and arm64), and cross-builds all release targets. Every action is pinned to a commit SHA; `TestWorkflowActionsPinnedToSHA` fails otherwise.

## Fixtures

`testdata/` holds synthetic Claude Code and Codex logs in several languages, canned judge answers, and expected outputs:

- **`testdata/expected/`** holds outputs of the original Python research implementation, which the Go program was ported from. They are compared under that implementation's frozen prices.
- **`testdata/expected-go/`** holds Go-written expected outputs at the current default prices.
- **`testdata/rubric-v2.1-b636272b.md`** is the rubric file exactly as validated.

When an output changes on purpose, update the expected file in the same change and explain why in the pull request.

## Changes to what the judge sees

Drift results are only comparable while the judge input stays the same. Several tests pin it:

- **`TestTranscriptsMatchReference`** checks rendered transcripts, locations and the system prompt byte for byte.
- **`TestBundledRubricJudgedTextIsTheValidatedOne`** pins a hash of the judged text: the rubric's machine part (between the `BEGIN`/`END MACHINE RUBRIC` markers) plus the settings rows S1–S10. **`TestValidatedFixtureAndBundledRubricJudgeTheSameText`** compares that text with the validated rubric file.
- **The prompt is passed on stdin, never on the command line** (`TestPromptOnlyOnStdin`, `TestRealCLIProtocolsPromptOnStdin`).

**Explanatory changes are free.** You may edit the explanatory parts of the rubric (Part A prose, Part C) freely. The version id shown to users is a hash of the whole file, so it changes; the judged-text tests show that the judge input did not.

**Changes to the judge input need re-validation.** That covers the machine part, the settings rows, the prompt wrapper and the renderer. Such a change needs a new rubric version and a re-validation run before release. Please open an issue first, and do not update the pinned hash in the same pull request without the validation results.

## Price table

Prices live in `internal/assets/prices.json`, in USD per million tokens. Each model row needs:

- `id` and `provider`
- `input`, `cache_read` and `output`
- for Anthropic models, `cache_write_5m` and `cache_write_1h`; for OpenAI models, the cache-write rate and, where the provider lists them, long-context rates and the cache lifetime
- `source`: the provider's official price page
- `read`: the date you read it

`TestPriceTableRowsHaveSources` checks the rows. A price change changes the leak amounts, so update `testdata/expected-go/` in the same change. Users can override the table without a release with `agent-leeway prices --edit`.

## Advice texts

The leak report's "change this" advice is part of the program's messages (`internal/messages`). When a catalogue of advice with sources is added, each entry should cite its source in the same way price rows do. Until then, please link the documentation that supports a changed advice text in your pull request.

## Pull requests

- Keep changes focused. Include tests for behavior changes.
- Use synthetic data only, in code, tests, issues and pull requests.
- Public documents state what the tool does and what was measured. Please do not add claims that are not backed by a test or a measurement.
