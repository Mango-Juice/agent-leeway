# Install

Leeway is one executable. It needs nothing else installed: no Go, Python or Node. For the in-depth check you also need the official `claude` or `codex` CLI, installed and logged in.

## One-line installer

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.sh | sh
```

- Downloads the archive for your system from GitHub Releases and checks it against `checksums.txt` (SHA-256). On a mismatch it installs nothing.
- Puts the executable in `~/.local/bin` and the license texts in `~/.local/share/agent-leeway`. No sudo.
- `LEEWAY_VERSION=v0.1.0` installs a given version. `LEEWAY_INSTALL_DIR=<folder>` installs somewhere else.

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex
```

- Downloads the zip for your system (x64 or ARM64) and checks it against `checksums.txt`.
- Puts `agent-leeway.exe` in `%LOCALAPPDATA%\Programs\agent-leeway` and adds that folder to your user Path. No administrator rights; the machine Path is not changed.
- `$env:LEEWAY_VERSION` and `$env:LEEWAY_INSTALL_DIR` work the same way.

Both installers then add the skill to every agent they find (a `~/.claude` or `~/.codex` folder), and print one line per agent with the undo command, for example `Claude Code: added the skill: <path> (undo: agent-leeway uninstall-skills --claude)`.

- If neither folder exists, they print `Skills: not added. No Claude Code or Codex found on this computer …` and the program is still installed. Run `agent-leeway install-skills` after you install Claude Code or Codex.
- To install the program only, set `LEEWAY_NO_SKILLS=1` (PowerShell: `$env:LEEWAY_NO_SKILLS = '1'`).
- Running the installer again leaves the skills as they are, or updates them.

## Homebrew (macOS and Linux)

This repository is also the tap:

```sh
brew tap mango-juice/agent-leeway https://github.com/Mango-Juice/agent-leeway
brew install mango-juice/agent-leeway/agent-leeway
```

Then run `agent-leeway install-skills` once to add the skill. Update with `brew upgrade agent-leeway`. Remove with `brew uninstall agent-leeway`, then `brew untap mango-juice/agent-leeway`.

## Scoop (Windows)

This repository is also the bucket:

```powershell
scoop bucket add agent-leeway https://github.com/Mango-Juice/agent-leeway
scoop install agent-leeway
```

Then run `agent-leeway install-skills` once to add the skill. Update with `scoop update agent-leeway`. Remove with `scoop uninstall agent-leeway`.

Use one install method per computer. An executable installed by the one-line installer can come first on your PATH and hide the Homebrew or Scoop one.

## Manual download

1. Download the archive for your system and `checksums.txt` from the [Releases page](https://github.com/Mango-Juice/agent-leeway/releases).
2. Check the archive: `shasum -a 256 -c checksums.txt --ignore-missing` on macOS and Linux, or `Get-FileHash <file> -Algorithm SHA256` on Windows, compared with the line in `checksums.txt`.
3. Optionally check where it was built: `gh attestation verify <file> -R Mango-Juice/agent-leeway`.
4. Put the executable on your PATH.

To build from source, see [development.md](development.md).

## npx skills

If you use the `skills` CLI, `npx skills add Mango-Juice/agent-leeway -g` copies the skill into your user folders. It adds the skill only; install the program with one of the methods above.

## The skill

```sh
agent-leeway install-skills            # every agent found on this computer
agent-leeway install-skills --claude   # Claude Code: ~/.claude/skills/agent-leeway/ (or under CLAUDE_CONFIG_DIR)
agent-leeway install-skills --codex    # Codex: ~/.agents/skills/agent-leeway/
agent-leeway install-skills --dry-run  # show what would change
```

- Each agent gets its own copy. The Claude Code copy judges only Claude Code sessions, and the Codex copy only Codex sessions, so a check started in one tool never uses the other provider's plan.
- The skill tells the agent to run `agent-leeway` commands, so it works the same in bash, zsh and PowerShell.
- An existing skill with the same name, or a copy you edited, is backed up first.

## Windows

- Windows 10 and 11, x64 and ARM64, with native Claude Code and Codex. Output is plain text, and Korean and other non-English text display correctly.
- Releases are not code-signed. SmartScreen may show "Windows protected your PC" (choose *More info*, then *Run anyway*). Smart App Control on Windows 11 may block the program until it has built reputation.
- If you run Claude Code or Codex inside WSL, their logs are in the Linux home. Install the Linux build inside WSL with the curl line. The Windows build does not read logs inside WSL.
- The tests and the installer run on Windows x64 and ARM64 CI runners, under PowerShell 5.1 and 7.

## Updates

`agent-leeway self-update --check` asks GitHub for the latest release and prints the line to run. It downloads nothing.

## Removal

1. Run `agent-leeway uninstall-skills`. It removes only the skill files it wrote, and restores a `SKILL.md` it had replaced. A copy you edited stays.
2. Delete the executable: `~/.local/bin/agent-leeway` and `~/.local/share/agent-leeway` on macOS and Linux, or the folder `%LOCALAPPDATA%\Programs\agent-leeway` on Windows. On Windows, also remove that folder from your user Path.
3. If you like, delete the cache and settings folders that `agent-leeway privacy` shows.
