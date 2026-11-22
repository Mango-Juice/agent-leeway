# NOTICE and attribution (draft)

Leeway (`agent-leeway`) is released under the MIT License (see LICENSE). This file lists the
third-party code compiled into the program, and the data and publications used to **validate** the tool or
to justify its default rubric. **No third-party dataset is redistributed** in this repository: only
aggregate numbers derived from them appear in the documents.

> Draft for owner review. Each entry states the license as shown in the module or on the source page on
> the check date (2026-11-22). Re-check before publishing.

## Third-party code in the executable

The program is built with Go and links the following modules. Their license texts are in each module's
source (`LICENSE` files) and must accompany binary distributions.

| Module (version in go.mod) | License | Notes |
|---|---|---|
| Go standard library and runtime (Go 1.27.4), including its vendored `golang.org/x/crypto`, `x/net` and `x/text` packages | BSD-3-Clause ("Copyright 2009 The Go Authors") | Compiled into every Go binary |
| `github.com/klauspost/compress` v1.20.2 (packages `zstd`, `fse`, `huff0`, `internal/le`, `internal/snapref`) | BSD-3-Clause ("Copyright (c) 2012 The Go Authors", "Copyright (c) 2019 Klaus Post") | The module's `gzhttp` part is Apache-2.0 but is not used |
| `github.com/klauspost/compress/zstd/internal/xxhash` | MIT ("Copyright (c) 2016 Caleb Spare") | Separate license file in that folder |
| `golang.org/x/text` v0.44.0 (packages `unicode/norm`, `transform`) | BSD-3-Clause ("Copyright 2009 The Go Authors") | |
| `golang.org/x/sys` v0.50.0 (package `windows`, Windows builds only) | BSD-3-Clause ("Copyright 2009 The Go Authors") | |

Every release archive contains `LICENSE`, this `NOTICE.md` and `THIRD_PARTY_LICENSES.txt`, which holds the
full license texts of everything above. That file is generated from the build (`go run ./internal/licenses/gen`),
and a test fails when it no longer matches `go.mod`. The install scripts copy the three files next to the
program: `~/.local/share/agent-leeway` on macOS and Linux, `%LOCALAPPDATA%\Programs\agent-leeway` on Windows.

## Datasets used for validation (not redistributed)

| Dataset | License (as stated by source) | How it was used | Attribution |
|---|---|---|---|
| Replication package of "How Coding Agents Fail Their Users" (Tang et al., 2026), https://github.com/ND-SaNDwichLAB/coding-agent-misalignment | MIT (repository license) | Episode labels (LLM-extracted and LLM-annotated, with a human audit) were the reference for the public-data check of drift-event recall. Only aggregate recall/AUC figures are reported. | Cite the paper (arXiv 2605.29442). If any label file is ever copied into this repository, its MIT copyright notice must be included with it. |
| SpecStory session histories in public GitHub repositories, referenced by that replication package | Per-repository. Only repositories under permissive licenses were used | Session text was fetched locally to run the diagnoser and was not stored in or redistributed by this repository | Report aggregate results only. Do not publish per-repository transcripts or excerpts. |
| nvidia/Open-SWE-Traces, https://huggingface.co/datasets/nvidia/Open-SWE-Traces | CC BY 4.0 | Validation of the leak (waste) metrics against task outcomes. Only per-trajectory numbers were computed locally, and only aggregates are reported. | "Contains information derived from nvidia/Open-SWE-Traces, licensed under CC BY 4.0." Cite the dataset and its paper (arXiv 2606.16038). Indicate that results are derived and modified. |

Considered but **not used**: SWE-chat (ODC-By, gated). It is cited only as literature.

## Publications cited by the rubric

The rubric's evidence grades cite published papers, preprints and public guideline documents.
The full list, with what was read (full text, partial, abstract or snippet), is in
[docs/references.md](docs/references.md). Citing these works does not imply their endorsement.

## Model providers

The drift diagnosis runs on the user's own Claude Code or Codex CLI. Leeway is not
affiliated with Anthropic or OpenAI. Product names are used only to describe compatibility.
