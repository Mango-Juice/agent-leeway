# Security policy

## Reporting a vulnerability

Please report vulnerabilities **privately**, not in a public issue:

1. Open the repository on GitHub: https://github.com/Mango-Juice/agent-leeway
2. Go to the **Security** tab and choose **Report a vulnerability** (GitHub private vulnerability reporting).

Describe the problem and how to reproduce it **with synthetic data**. Do not attach real session logs, transcripts, reports or instruction files. They can contain your code, prompts and secrets. If a real log is the only way to show the problem, say so in the report, and we will agree on a way to share a minimal, redacted excerpt.

## What is in scope

Leeway reads private data and runs other programs, so these matter most:

- **Local data handling.**
  - Reading Claude Code and Codex logs, including compressed Codex logs.
  - The cache and report folders and their permissions.
  - Anything that could expose log content to other local users or write outside the tool's own folders.
- **What leaves the computer.** The in-depth check sends each selected session to that session's own provider through the official `claude` or `codex` CLI. Anything that sends data elsewhere, sends more than intended, or sends a session to the wrong provider is in scope.
- **Running the judge CLIs.**
  - The judge lock-down is enforced through CLI flags (no tools, no settings, hooks or plugins, read-only sandbox). A way around it is in scope.
  - So are argument injection through `.cmd`/`.bat` shims on Windows, and child processes that outlive a timeout or cancel.
- **Prompt injection from session content** that makes the judge take an action. A transcript that only steers the judge toward a lenient or harsh verdict is a rubric-quality issue; please report it as a "wrong diagnosis" issue instead.
- **Install scripts and the skill.**
  - Checksum verification in `install.sh` and `install.ps1`.
  - Changes to PATH: on Windows, only the user Path may change.
  - `install-skills` / `uninstall-skills` writing or deleting files they should not.
- **Release artifacts.** Checksums and build provenance attestations.

Out of scope: vulnerabilities in Claude Code, Codex or the model providers themselves. Please report those to their vendors.

## What to expect

- We reply in the private report and agree on a fix and disclosure timeline with you.
- Fixes are released as a new version. The advisory is published when a fixed release is available.
- With your consent, you are credited in the advisory.

## Supported versions

Only the latest release receives security fixes.
