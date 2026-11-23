#!/bin/sh
# Builds the example report site from made-up logs with the real binary (site/README.md).
#   site/build.sh [BINARY] [OUT]
# BINARY: the agent-leeway binary to render with (default: build ./cmd/agent-leeway, which needs Go);
# OUT: site folder (default: ./site-out). Needs python3 3.7 or later.
# Runs offline: the judge is the hand-written stand-in in example.py (LEEWAY_JUDGE_CMD); no model is called.
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
BIN=${1:-}
OUT=${2:-$ROOT/site-out}
# the stand-in judge runs from the work folder under /tmp: LEEWAY_JUDGE_CMD is split on spaces, so the project folder
# may have spaces in its path but the judge's may not
WORK=$(mktemp -d "/tmp/leeway-example.XXXXXX")
trap 'rm -rf "$WORK"' EXIT
cp "$ROOT/site/example.py" "$ROOT/site/frame.html" "$WORK/"
PY=$(command -v python3) || { echo "site/build.sh: python3 is needed" >&2; exit 1; }
case "$PY" in *" "*) echo "site/build.sh: python3 is at a path with a space ($PY); put one without a space first on PATH" >&2; exit 1;; esac
"$PY" -c 'import sys; sys.exit(sys.version_info < (3, 7))' || { echo "site/build.sh: python3 3.7 or later is needed" >&2; exit 1; }
if [ -z "$BIN" ]; then BIN=$WORK/agent-leeway; (cd "$ROOT" && go build -o "$BIN" ./cmd/agent-leeway); fi
# one instant for the made-up logs and the binary (LEEWAY_NOW), so a rebuild with the same date is byte-identical
NOW=${LEEWAY_EXAMPLE_NOW:-$(date -u +%Y-%m-%dT12:00:00Z)}
mkdir -p "$WORK/raw"
for L in en ko; do
  H=$WORK/$L/home; mkdir -p "$H/.claude"
  # the made-up person has kept Claude Code transcripts for 90 days (cleanupPeriodDays) since before the logs began,
  # so the previous 30 days count as covered: the logs start on day -57 because that is when they began, not because
  # some were deleted. The file is dated 100 days back: a setting changed later would not count.
  printf '{"cleanupPeriodDays": 90}\n' >"$H/.claude/settings.json"
  "$PY" -c 'import os,sys,datetime as d; t=d.datetime.fromisoformat(sys.argv[2].replace("Z","+00:00")).timestamp()-100*86400; os.utime(sys.argv[1],(t,t))' "$H/.claude/settings.json" "$NOW"
  "$PY" "$WORK/example.py" logs "$WORK/$L/logs" --lang $L --now "$NOW"
  # a clean environment with an isolated home: nothing is read from or written to the developer's own settings,
  # logs or cache (CLAUDE_CONFIG_DIR, CODEX_HOME, XDG_*, LEEWAY_* of the caller do not reach the binary)
  run() { env -i PATH="$PATH" HOME="$H" LEEWAY_NOW="$NOW" "$@"; }
  run LEEWAY_EXAMPLE_LANG=$L LEEWAY_JUDGE_CMD="$PY $WORK/example.py judge" \
    "$BIN" drift --claude-dir "$WORK/$L/logs/claude" --codex-dir "$WORK/$L/logs/codex" --cache "$H/cache" \
      --since 60d --max-sessions 0 --concurrency 1 --yes --lang $L >"$WORK/$L/drift.txt" 2>&1 || { cat "$WORK/$L/drift.txt"; exit 1; }
  run "$BIN" report --claude-dir "$WORK/$L/logs/claude" --codex-dir "$WORK/$L/logs/codex" --cache "$H/cache" \
      --since 30d --lang $L --out "$WORK/$L/report" >"$WORK/$L/report.txt" 2>&1 || { cat "$WORK/$L/report.txt"; exit 1; }
  mkdir -p "$OUT/report"
  cp "$(ls "$WORK/$L/report"/*.html | head -1)" "$OUT/report/$L.html"   # the report, byte for byte as the binary wrote it
  cp "$(ls "$WORK/$L/report"/*.md | head -1)" "$WORK/raw/$L.md"
  cp "$WORK/$L/report.txt" "$WORK/raw/$L.terminal.txt"
  cp "$H/cache/drift/results.jsonl" "$WORK/raw/$L.results.jsonl"
  if [ -n "${LEEWAY_EXAMPLE_KEEP:-}" ]; then rm -rf "$LEEWAY_EXAMPLE_KEEP/$L"; mkdir -p "$LEEWAY_EXAMPLE_KEEP"; cp -R "$WORK/$L" "$LEEWAY_EXAMPLE_KEEP/$L"; fi
done
"$PY" "$WORK/example.py" frame "$OUT"
# the binary writes reports readable by their owner only; the site is public, so everything in it is world-readable
chmod -R a+rX "$OUT"
"$PY" "$WORK/example.py" check "$OUT" "$WORK/raw"
echo "example site: $OUT"
