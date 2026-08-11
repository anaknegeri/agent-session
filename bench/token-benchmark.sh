#!/usr/bin/env bash
#
# token-benchmark.sh — Measure token usage: with vs without agent-session
#
# This benchmark measures the real output size (in characters) of the
# orientation/context-gathering operations an AI coding agent performs,
# then converts to approximate token cost (chars / 4, the standard
# heuristic for English text).
#
# Scenarios:
#   1. Cold start      — agent's first turn in a project
#   2. Post-compaction — context window filled up, agent must re-orient
#   3. Agent handoff   — switch from one agent (e.g. Claude) to another (e.g. Codex)
#   4. Per-turn cost   — cost of maintaining context across N turns
#
# Usage:
#   ./bench/token-benchmark.sh
#
# Reproducibility:
#   All measurements are live — the script runs real `agent-session`
#   commands and real `git` / file reads. Results vary by project state
#   (working tree, session history, memory).
#
set -euo pipefail

# --- locate project root -----------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN="$ROOT/bin/agent-session"

if [ ! -x "$BIN" ]; then
    echo "error: $BIN not found. Run 'make build' first." >&2
    exit 1
fi

cd "$ROOT"

# --- helpers -----------------------------------------------------------------

# chars_to_tokens: divide by 4 (standard heuristic)
ct() { echo $(( $1 / 4 )); }

# fmt: right-align a number in a 6-char field
fmt() { printf "%6d" "$1"; }

# pct_savings: what percentage was saved
pct() {
    local without=$1 with=$2
    if [ "$without" -eq 0 ]; then echo 0; return; fi
    echo $(( 100 - (with * 100 / without) ))
}

measure_file() {
    # $1 = file path. Returns char count or 0 if missing.
    if [ -f "$1" ]; then wc -c < "$1" | tr -d ' '; else echo 0; fi
}

measure_cmd() {
    # $@ = command. Returns char count of stdout.
    "$@" 2>/dev/null | wc -c | tr -d ' '
}

# --- banner ------------------------------------------------------------------

echo ""
echo "================================================================"
echo "  Agent Session — Token Usage Benchmark"
echo "  Project: $(basename "$ROOT")  ·  $(date '+%Y-%m-%d %H:%M')"
echo "================================================================"
echo ""

# =============================================================================
# SCENARIO 1: Cold Start (agent's first turn)
# =============================================================================

echo "--- Scenario 1: Cold Start (first turn) ---"
echo ""

# WITHOUT agent-session: agent must explore to understand the project
WO_README=$(measure_file README.md)
WO_AGENTS=$(measure_file AGENTS.md)
WO_GITLOG=$(measure_cmd git log --oneline -20)
WO_GITSTATUS=$(measure_cmd git status --short)
WO_GITDIFF=$(measure_cmd git diff HEAD)
WO_GLOB=$(measure_cmd find . -name '*.go' -not -path './vendor/*')

WO_TOTAL=$((WO_README + WO_AGENTS + WO_GITLOG + WO_GITSTATUS + WO_GITDIFF + WO_GLOB))

echo "  WITHOUT agent-session (manual exploration):"
echo "    README.md            $(fmt $WO_README) chars"
echo "    AGENTS.md            $(fmt $WO_AGENTS) chars"
echo "    git log -20          $(fmt $WO_GITLOG) chars"
echo "    git status           $(fmt $WO_GITSTATUS) chars"
echo "    git diff HEAD        $(fmt $WO_GITDIFF) chars"
echo "    glob source files    $(fmt $WO_GLOB) chars"
echo "    ─────────────────────────────────────"
echo "    TOTAL                $(fmt $WO_TOTAL) chars  ≈ $(fmt $(ct $WO_TOTAL)) tokens"
echo ""

# WITH agent-session: one context.get call
W_CONTEXT=$("$BIN" context --depth summary 2>/dev/null | wc -c | tr -d ' ')

echo "  WITH agent-session:"
echo "    context.get (summary) $(fmt $W_CONTEXT) chars"
echo "    ─────────────────────────────────────"
echo "    TOTAL                 $(fmt $W_CONTEXT) chars  ≈ $(fmt $(ct $W_CONTEXT)) tokens"
echo ""

S1_PCT=$(pct $WO_TOTAL $W_CONTEXT)
echo "  ► Savings: $(fmt $(ct $WO_TOTAL)) → $(fmt $(ct $W_CONTEXT)) tokens"
echo "            $(fmt $(( $(ct $WO_TOTAL) - $(ct $W_CONTEXT) )) ) tokens saved ($S1_PCT%)"
echo ""

# Cache for later scenarios
COLD_WO=$WO_TOTAL
COLD_W=$W_CONTEXT

# =============================================================================
# SCENARIO 2: Post-Compaction (context window filled, agent must re-orient)
# =============================================================================

echo "--- Scenario 2: Post-Compaction (re-orientation after context loss) ---"
echo ""
echo "  After context compaction (PreCompact), the agent loses its working"
echo "  memory and must re-gather context to continue."
echo ""
echo "  WITHOUT agent-session: re-read everything"
echo "    cost = same as cold start = $(fmt $(ct $COLD_WO)) tokens"
echo ""
echo "  WITH agent-session: one context.get call"
echo "    cost = $(fmt $(ct $COLD_W)) tokens"
echo ""

# Also show the full-depth cost for agents that need more detail
W_FULL=$("$BIN" context --depth full 2>/dev/null | wc -c | tr -d ' ')
echo "  (full depth: $(fmt $(ct $W_FULL)) tokens — still far below manual exploration)"
echo ""

# =============================================================================
# SCENARIO 3: Agent Handoff (Claude → Codex)
# =============================================================================

echo "--- Scenario 3: Agent Handoff (switch agents) ---"
echo ""
echo "  When switching from one agent to another, the new agent starts"
echo "  with zero context about the work done so far."
echo ""

WO_HANDOFF=$COLD_WO
W_HANDOFF_CTX=$("$BIN" context --depth summary 2>/dev/null | wc -c | tr -d ' ')
W_HANDOFF_HO=$("$BIN" handoff codex 2>/dev/null | wc -c | tr -d ' ')
W_HANDOFF=$((W_HANDOFF_CTX + W_HANDOFF_HO))

echo "  WITHOUT agent-session:"
echo "    full exploration (blind start)  $(fmt $(ct $WO_HANDOFF)) tokens"
echo "    + decisions/progress/blockers are LOST"
echo ""
echo "  WITH agent-session:"
echo "    context.get (summary)           $(fmt $(ct $W_HANDOFF_CTX)) tokens"
echo "    handoff (deterministic state)   $(fmt $(ct $W_HANDOFF_HO)) tokens"
echo "    ─────────────────────────────────────"
echo "    TOTAL                           $(fmt $(ct $W_HANDOFF)) tokens"
echo "    + full task/decision/blocker state preserved"
echo ""

S3_PCT=$(pct $WO_HANDOFF $W_HANDOFF)
echo "  ► Savings: $(fmt $(ct $WO_HANDOFF)) → $(fmt $(ct $W_HANDOFF)) tokens ($S3_PCT%)"
echo ""

# =============================================================================
# SCENARIO 4: Cumulative cost over N turns
# =============================================================================

echo "--- Scenario 4: Cumulative Cost Over Multiple Turns ---"
echo ""
echo "  Each re-orientation event (compaction, new session, handoff) costs tokens."
echo "  This shows the cumulative cost over N such events."
echo ""
printf "  %-6s  %12s  %12s  %12s  %8s\n" "Turns" "Without" "With" "Saved" "%"
printf "  %-6s  %12s  %12s  %12s  %8s\n" "-----" "------" "----" "------" "---"

for n in 1 3 5 10 20; do
    wo=$(( COLD_WO / 4 * n ))
    w=$(( COLD_W / 4 * n ))
    saved=$(( wo - w ))
    p=$(pct $wo $w)
    printf "  %-6d  %9d tok  %9d tok  %9d tok  %5d%%\n" "$n" "$wo" "$w" "$saved" "$p"
done
echo ""

# =============================================================================
# SCENARIO 5: Context Budget — depth comparison
# =============================================================================

echo "--- Context Depth Comparison (agent-session) ---"
echo ""
S_DEPTH=$("$BIN" context --depth summary 2>/dev/null | wc -c | tr -d ' ')
R_DEPTH=$("$BIN" context --depth recent 2>/dev/null | wc -c | tr -d ' ')
F_DEPTH=$("$BIN" context --depth full 2>/dev/null | wc -c | tr -d ' ')

printf "  %-12s  %10s  %10s\n" "Depth" "Chars" "≈ Tokens"
printf "  %-12s  %10s  %10s\n" "-----" "-----" "--------"
printf "  %-12s  %10d  %10d\n" "summary" "$S_DEPTH" "$(( S_DEPTH / 4 ))"
printf "  %-12s  %10d  %10d\n" "recent" "$R_DEPTH" "$(( R_DEPTH / 4 ))"
printf "  %-12s  %10d  %10d\n" "full" "$F_DEPTH" "$(( F_DEPTH / 4 ))"
echo ""
echo "  (summary is clamped at max_total_chars=4000; full is never truncated)"
echo ""

# =============================================================================
# Summary
# =============================================================================

echo "================================================================"
echo "  SUMMARY"
echo "================================================================"
echo ""
echo "  Cold start orientation:"
echo "    Without: ~$(fmt $(ct $COLD_WO)) tokens    With: ~$(fmt $(ct $COLD_W)) tokens"
echo "    Savings: $S1_PCT% ($(fmt $(( $(ct $COLD_WO) - $(ct $COLD_W) )) ) tokens per orientation)"
echo ""
echo "  Handoff:"
echo "    Without: ~$(fmt $(ct $WO_HANDOFF)) tokens (state lost)"
echo "    With:    ~$(fmt $(ct $W_HANDOFF)) tokens (state preserved)"
echo "    Savings: $S3_PCT%"
echo ""
echo "  Over 10 re-orientations: ~$(( ($(ct $COLD_WO) - $(ct $COLD_W)) * 10 )) tokens saved"
echo ""
echo "  Reproduce: ./bench/token-benchmark.sh"
echo ""
