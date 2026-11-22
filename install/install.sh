#!/bin/sh
# agent-leeway installer for macOS and Linux.
#   curl -fsSL https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.sh | sh
# Downloads the release archive for this system from GitHub Releases, checks it against checksums.txt and
# installs the single executable into ~/.local/bin (or $LEEWAY_INSTALL_DIR) and its license texts into
# ~/.local/share/agent-leeway, then runs `agent-leeway install-skills --auto`, which adds the skill for Claude Code
# and Codex if their folders exist (a SKILL.md it did not write is backed up first). No sudo, nothing else changes.
# Optional: LEEWAY_VERSION=v0.1.0 (default: latest), LEEWAY_RELEASE_BASE=<url> (another release location),
# LEEWAY_NO_SKILLS=1 (install the program only).
set -eu

# The whole script is a function called on the last line: a truncated download (curl | sh) runs nothing.
main() {

  OWNER="Mango-Juice"   # keep in sync with internal/meta/meta.go
  REPO="agent-leeway"
  NAME="agent-leeway"
  VERSION="${LEEWAY_VERSION:-latest}"
  DEST="${LEEWAY_INSTALL_DIR:-$HOME/.local/bin}"

  if [ -n "${LEEWAY_RELEASE_BASE:-}" ]; then
    BASE="$LEEWAY_RELEASE_BASE"
  elif [ "$VERSION" = "latest" ]; then
    BASE="https://github.com/$OWNER/$REPO/releases/latest/download"
  else
    BASE="https://github.com/$OWNER/$REPO/releases/download/$VERSION"
  fi

  fail() { echo "$NAME install: $*" >&2; exit 1; }

  if [ "$(id -u 2>/dev/null || echo 1)" = "0" ] && [ -z "${LEEWAY_INSTALL_DIR:-}" ]; then
    echo "$NAME install: running as root installs into root's $DEST, which your own account will not find." >&2
    echo "  Run it without sudo, or set LEEWAY_INSTALL_DIR." >&2
  fi

  case "$(uname -s)" in
    Darwin) OS=darwin ;;
    Linux) OS=linux ;;
    *) fail "unsupported system $(uname -s); on Windows use install.ps1" ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *) fail "unsupported processor $(uname -m)" ;;
  esac

  ARCHIVE="${NAME}_${OS}_${ARCH}.tar.gz"
  TMP="$(mktemp -d 2>/dev/null || mktemp -d -t "$NAME")"
  trap 'rm -rf "$TMP"' EXIT
  trap 'rm -rf "$TMP"; exit 130' INT TERM

  fetch() {
    if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"
    elif command -v wget >/dev/null 2>&1; then wget -q "$1" -O "$2"
    else fail "curl or wget is needed"; fi
  }

  echo "Downloading $ARCHIVE ..."
  fetch "$BASE/$ARCHIVE" "$TMP/$ARCHIVE" || fail "download failed: $BASE/$ARCHIVE"
  fetch "$BASE/checksums.txt" "$TMP/checksums.txt" || fail "download failed: $BASE/checksums.txt"

  WANT="$(awk -v f="$ARCHIVE" '$2 == f || $2 == "*"f {print $1}' "$TMP/checksums.txt")"
  [ -n "$WANT" ] || fail "$ARCHIVE is not listed in checksums.txt"
  if command -v sha256sum >/dev/null 2>&1; then GOT="$(sha256sum "$TMP/$ARCHIVE" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then GOT="$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print $1}')"
  else fail "sha256sum or shasum is needed to verify the download"; fi
  [ "$WANT" = "$GOT" ] || fail "checksum mismatch for $ARCHIVE (expected $WANT, got $GOT); nothing was installed"

  tar -xzf "$TMP/$ARCHIVE" -C "$TMP" || fail "could not unpack $ARCHIVE"
  [ -f "$TMP/$NAME" ] || fail "$ARCHIVE does not contain $NAME"
  mkdir -p "$DEST"
  cp "$TMP/$NAME" "$DEST/$NAME.tmp"
  chmod 755 "$DEST/$NAME.tmp"
  mv -f "$DEST/$NAME.tmp" "$DEST/$NAME"
  echo "Installed $DEST/$NAME"

  # license texts that come with the program (its own and the third-party ones compiled into it)
  DOCS="${XDG_DATA_HOME:-$HOME/.local/share}/$NAME"
  mkdir -p "$DOCS"
  for f in LICENSE NOTICE.md THIRD_PARTY_LICENSES.txt; do
    [ -f "$TMP/$f" ] && cp "$TMP/$f" "$DOCS/$f"
  done
  echo "License texts: $DOCS"

  ONPATH=1
  case ":$PATH:" in
    *":$DEST:"*) ;;
    *) ONPATH=0
       echo "Note: $DEST is not on your PATH. Add this line to your shell profile:"
       echo "  export PATH=\"$DEST:\$PATH\"" ;;
  esac

  # The skill for Claude Code and Codex. Never fails the install: the program is in place either way.
  # install-skills --auto: 0 = written or up to date, 3 = no agent folder found (it says so), else an error.
  SKILLS=0
  if [ -n "${LEEWAY_NO_SKILLS:-}" ] && [ "${LEEWAY_NO_SKILLS}" != "0" ]; then
    echo "Skills: not added (LEEWAY_NO_SKILLS is set)."
  else
    RC=0
    if "$DEST/$NAME" install-skills -h 2>&1 | grep -q -e '-auto'; then
      "$DEST/$NAME" install-skills --auto || RC=$?
    else
      # a release from before --auto (v0.1.0): plain install-skills, whose "none found" message means 3 here
      OUT=$("$DEST/$NAME" install-skills 2>&1) || RC=$?
      case "$OUT" in
        *"Neither Claude Code nor Codex was found"*)
          RC=3
          echo "Skills: not added. No Claude Code or Codex found on this computer (no ~/.claude or ~/.codex folder)." ;;
        *) [ -z "$OUT" ] || printf '%s\n' "$OUT" | grep -v '^Ask your agent for a' || true ;;
      esac
    fi
    if [ "$RC" = 0 ]; then SKILLS=1
    elif [ "$RC" != 3 ]; then
      echo "Skills: not added (see the message above). The program is installed; run \`$NAME install-skills\` to try again."
    fi
  fi

  CMD="$NAME"
  [ "$ONPATH" = 1 ] || CMD="$DEST/$NAME"
  if [ "$SKILLS" = 1 ]; then
    echo "Next: ask Claude Code or Codex for a \"leeway report\". In a terminal instead: $CMD report"
  else
    echo "Next: run \`$CMD install-skills\` once (after installing Claude Code or Codex, if neither is here), then ask Claude Code or Codex for a \"leeway report\". In a terminal instead: $CMD report"
  fi
}

main "$@"
