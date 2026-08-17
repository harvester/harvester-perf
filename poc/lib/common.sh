# shellcheck shell=bash
# Shared helpers for harvester-perf: logging, run layout, JSON/Markdown emit.

PERF_HOME="${PERF_HOME:-/opt/harvester-perf}"
RESULTS_DIR="${RESULTS_DIR:-/results}"

if [ -z "${NO_COLOR:-}" ]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BLUE=$'\033[36m'
else
  C_RESET=''; C_BOLD=''; C_DIM=''; C_RED=''; C_GREEN=''; C_YELLOW=''; C_BLUE=''
fi

now_ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
_stamp() { date -u +%H:%M:%S; }
epoch_ms() { date -u +%s%3N; }

info() { printf '%s %s\n' "${C_DIM}$(_stamp)${C_RESET}" "$*"; }
step() { printf '\n%s\n' "${C_BOLD}${C_BLUE}==> $*${C_RESET}"; }
ok()   { printf '  %s %s\n' "${C_GREEN}v${C_RESET}" "$*"; }
warn() { printf '  %s %s\n' "${C_YELLOW}!${C_RESET}" "$*"; }
err()  { printf '  %s %s\n' "${C_RED}x${C_RESET}" "$*" >&2; }
die()  { err "$*"; exit 1; }

# jq with the harvester-perf jq library on the include path.
jqp() { jq -L "$PERF_HOME/lib" "$@"; }

# emit_json <suite> <file-or-stdin>: store a suite's machine readable result.
emit_json() {
  local suite="$1"
  jq . > "$RUN_DIR/${suite}.json"
}

# md <suite> ...: append a line to a suite's markdown fragment.
md() { printf '%s\n' "$*" >> "$RUN_DIR/${MD_SUITE:?MD_SUITE not set}.md"; }

# md_init <suite> <title>
md_init() {
  MD_SUITE="$1"
  : > "$RUN_DIR/${MD_SUITE}.md"
  md "## $2"
  md ""
}

# Convert seconds (float) to a short human string.
human_secs() { awk -v s="$1" 'BEGIN{ if (s<60) printf "%.1fs", s; else printf "%dm%02ds", int(s/60), s%60 }'; }

# Create a scratch directory that is removed when the caller exits.
mktmp() { mktemp -d "${TMPDIR:-/tmp}/harvester-perf.XXXXXX"; }
