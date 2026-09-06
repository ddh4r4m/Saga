#!/bin/bash
# Three live probes against the pinned Claude Code, for readiness rows 9,
# 10 and 17 of docs/12. Cents, not dollars: one turn each, capped at
# --max-turns 4 and --max-budget-usd 0.10.
#
#   P9   Is a PreToolUse deny in JSON honoured when the hook exits
#        non-zero? Open item 5. The control is exit 2 with plain stderr,
#        the documented block path.
#   P10  Do the hooks actually fire? One turn, then `saga doctor` must
#        see the tool_call, the tool_result and the Stop claim event in
#        that session. Also proves `saga uninstall --dry-run` lists
#        exactly what install wrote.
#   P17  Capture one real PostToolUseFailure payload and compare its
#        field set with harness-facts C34.
#
# Run this from a plain terminal, not from inside an agent session, and
# export CLAUDE_CODE_OAUTH_TOKEN (from `claude setup-token`) or
# ANTHROPIC_API_KEY first: a fresh CLAUDE_CONFIG_DIR has no login. The
# probes never read ~/.claude.
#
# Knobs: OUT (evidence root), MODEL (default sonnet), CLAUDE_BIN (for the
# dry run's stub), DRY_RUN=1 (skip the token and marker checks; used by
# the test that exercises this script without a model).
set -uo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${OUT:-/tmp/saga-harness-probes-$(date +%Y%m%d-%H%M%S)}
MODEL=${MODEL:-sonnet}
CLAUDE_BIN=${CLAUDE_BIN:-claude}
DRY_RUN=${DRY_RUN:-0}

if [ "$DRY_RUN" != "1" ]; then
  for m in SAGA_AGENT_SHELL CLAUDECODE CLAUDE_CODE_ENTRYPOINT CODEX_SANDBOX CODEX_CI GEMINI_CLI CURSOR_AGENT; do
    if [ -n "${!m:-}" ]; then
      echo "harness-probes: $m is set; run this from a plain terminal" >&2
      exit 6
    fi
  done
  if [ -z "${CLAUDE_CODE_OAUTH_TOKEN:-}" ] && [ -z "${ANTHROPIC_API_KEY:-}" ]; then
    echo "harness-probes: export CLAUDE_CODE_OAUTH_TOKEN (claude setup-token) or ANTHROPIC_API_KEY first" >&2
    exit 6
  fi
fi
command -v "$CLAUDE_BIN" >/dev/null || { echo "harness-probes: $CLAUDE_BIN not on PATH" >&2; exit 6; }
command -v python3 >/dev/null || { echo "harness-probes: python3 not on PATH" >&2; exit 6; }

mkdir -p "$OUT/bin"
go -C "$ROOT" build -o "$OUT/bin/saga" ./cmd/saga || { echo "harness-probes: cannot build saga" >&2; exit 6; }
SAGA="$OUT/bin/saga"

CLAUDE_VER=$("$CLAUDE_BIN" --version 2>&1 | head -1)
CLAUDE_SHA=$(shasum -a 256 "$(command -v "$CLAUDE_BIN")" 2>/dev/null | cut -d' ' -f1)
{
  echo "harness-probes $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "claude: $CLAUDE_VER"
  echo "claude binary sha256: ${CLAUDE_SHA:-unknown}"
  echo "saga: $("$SAGA" version 2>&1 | head -1)"
  echo "model: $MODEL   out: $OUT"
} | tee "$OUT/run.log"

# One private environment per probe: never the operator's ~/.claude.
probe_env() {  # cfg
  echo "HOME=$1/home"
  echo "CLAUDE_CONFIG_DIR=$1/claude-config"
  echo "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"
  echo "DISABLE_AUTOUPDATER=1"
  echo "DISABLE_TELEMETRY=1"
  echo "DISABLE_ERROR_REPORTING=1"
  echo "CI=1"
}

# new_session_id prints a fresh UUID. Claude Code refuses a --session-id
# that is not one ("Error: Invalid session ID. Must be a valid UUID.")
# and exits before writing a single stream event, which is how the
# 2026-09-06 owner run produced four empty logs and three verdicts read
# off nothing. The bench derives its id from the run seed
# (adapter.SessionID); a probe has no seed, so it draws one.
new_session_id() {
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr 'A-Z' 'a-z'
  else
    python3 -c "import uuid; print(uuid.uuid4())"
  fi
}

run_claude() {  # cfg workspace settings prompt logfile session-id
  local cfg=$1 ws=$2 settings=$3 prompt=$4 log=$5 session=$6
  local -a env_args=()
  while IFS= read -r kv; do env_args+=("$kv"); done < <(probe_env "$cfg")
  for k in PATH TMPDIR LANG SHELL USER LOGNAME TERM SSL_CERT_FILE \
           CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_BASE_URL \
           STUB_HONOUR_DENY STUB_TOOL_EXIT STUB_BAD_SESSION; do
    [ -n "${!k:-}" ] && env_args+=("$k=${!k}")
  done
  mkdir -p "$cfg/home" "$cfg/claude-config"
  printf '%s' "$prompt" | (cd "$ws" && env -i "${env_args[@]}" "$CLAUDE_BIN" \
    -p --output-format stream-json --verbose \
    --max-turns 4 --permission-mode acceptEdits --tools Bash \
    --settings "$settings" --session-id "$session" --max-budget-usd 0.10 \
    --model "$MODEL") > "$log" 2>"$log.err"
}

# transcript_ok: the harness got far enough for its log to mean
# anything. A probe reads a verdict off the stream, so a stream with no
# `system/init` and no `result` is not evidence of the thing probed: it
# is evidence the harness never started or never finished. Every verdict
# below is gated on this, so an invocation that dies on its arguments can
# never be reported as PASS or FAIL.
transcript_ok() {  # native.jsonl
  [ -s "$1" ] || return 1
  grep -q '"type":"system"' "$1" 2>/dev/null || return 1
  grep -q '"subtype":"init"' "$1" 2>/dev/null || return 1
  grep -q '"type":"result"' "$1" 2>/dev/null || return 1
  return 0
}

# why_no_transcript: the first line of the harness's stderr, which is
# where "Invalid session ID" and every other startup refusal appears.
why_no_transcript() {  # native.jsonl
  local err="$1.err" line=""
  [ -f "$err" ] && line=$(head -1 "$err" | tr -d '\r')
  if [ -z "$line" ]; then
    if [ -s "$1" ]; then line="the harness wrote a stream with no init or result event"
    else line="the harness wrote nothing and said nothing"; fi
  fi
  printf '%s' "$line"
}

results=()
say() { echo "$1" | tee -a "$OUT/run.log"; }

# ---------------------------------------------------------------- P9
# A PreToolUse hook that denies in JSON and exits non-zero. If the tool
# still runs, Claude Code fails open on a non-zero exit and every deny
# Saga writes has to use the exit-2 path instead.
probe_p9() {
  # Separate statements: bash 3.2 does not see an earlier name declared
  # in the same `local`, and set -u then reports it unbound.
  local variant=$1 hook_exit=$2 mode=$3
  local dir="$OUT/p9-$variant"
  mkdir -p "$dir/ws" "$dir/cfg"
  local hook="$dir/hook.sh"
  if [ "$mode" = "json" ]; then
    cat > "$hook" <<EOF
#!/bin/bash
cat > "$dir/hook-stdin.json"
printf '%s' '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"saga-probe-p9"}}'
exit $hook_exit
EOF
  else
    cat > "$hook" <<EOF
#!/bin/bash
cat > "$dir/hook-stdin.json"
echo "saga-probe-p9" >&2
exit $hook_exit
EOF
  fi
  chmod +x "$hook"
  python3 - "$dir/settings.json" "$hook" <<'PY'
import json, sys
json.dump({"hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": sys.argv[2], "timeout": 30}]}]}},
          open(sys.argv[1], "w"), indent=2)
PY
  run_claude "$dir/cfg" "$dir/ws" "$dir/settings.json" \
    'Run exactly one Bash command: echo saga-probe. Then stop.' \
    "$dir/native.jsonl" "$(new_session_id)"
  if ! transcript_ok "$dir/native.jsonl"; then
    echo "nolog nolog"
    return 0
  fi
  local ran=no
  grep -q '"name":"Bash"' "$dir/native.jsonl" 2>/dev/null && ran=yes
  # The reason has to be visible somewhere the model could have seen it:
  # in the stream, which carries the harness's own denial text. A tool
  # that simply was not called looks identical to a denied one without
  # this, which is why "did not run" alone is not a PASS.
  local saw_reason=no
  grep -q 'saga-probe-p9' "$dir/native.jsonl" 2>/dev/null && saw_reason=yes
  echo "$ran $saw_reason"
}

say ""
say "P9: is a PreToolUse deny honoured when the hook exits non-zero?"
read -r p9_ran p9_reason <<<"$(probe_p9 json-exit1 1 json)"
read -r ctl_ran ctl_reason <<<"$(probe_p9 text-exit2 2 text)"
if [ "$p9_ran" = "nolog" ] || [ "$ctl_ran" = "nolog" ]; then
  p9="INCONCLUSIVE"
  p9_note="the harness produced no usable transcript: $(why_no_transcript "$OUT/p9-json-exit1/native.jsonl")"
elif [ "$p9_ran" = "no" ] && [ "$p9_reason" = "yes" ]; then
  p9="PASS"; p9_note="JSON deny honoured on exit 1: the tool did not run and the reason is in the transcript"
elif [ "$p9_ran" = "no" ]; then
  # Not running is not the same as being denied: without the reason
  # anywhere in the stream, a model that never called the tool is
  # indistinguishable from a hook that blocked it.
  p9="INCONCLUSIVE"; p9_note="the tool did not run, but the deny reason is nowhere in the transcript; the model may simply not have called it"
elif [ "$ctl_ran" = "no" ]; then
  p9="FAIL"; p9_note="fail-open on exit 1: the tool ran. The documented exit-2 path did block, so every deny must use it"
else
  p9="INCONCLUSIVE"; p9_note="neither variant blocked; the hook may not have run at all (check $OUT/p9-*/hook-stdin.json)"
fi
say "  P9 $p9: $p9_note"
say "  evidence: $OUT/p9-json-exit1/native.jsonl and $OUT/p9-text-exit2/native.jsonl"
results+=("P9 $p9")

cat > "$OUT/harness-facts-proposed.md" <<EOF
<!-- proposed row for docs/specs/harness-facts.md; saga edits that file, not this script -->
| C-P9 | PreToolUse JSON decision on a non-zero hook exit | $CLAUDE_VER | $p9_note | probed $(date -u +%Y-%m-%d), scripts/harness-probes.sh, evidence in $OUT/p9-json-exit1/ and $OUT/p9-text-exit2/ |
EOF
say "  proposed harness-facts row: $OUT/harness-facts-proposed.md"

# --------------------------------------------------------------- P10
# Registered is not firing. Install the composed hook, drive one turn,
# then let doctor read what the session recorded.
say ""
say "P10: do the hooks fire, and does uninstall --dry-run list what install wrote?"
p10dir="$OUT/p10"; mkdir -p "$p10dir/ws" "$p10dir/cfg"
(cd "$p10dir/ws" && git init -q && git commit -q --allow-empty -m base 2>/dev/null)
"$SAGA" init --project "$p10dir/ws" >/dev/null 2>&1 || (cd "$p10dir/ws" && "$SAGA" init >/dev/null 2>&1)
install_out=$(cd "$p10dir/ws" && "$SAGA" install --harness claude-code 2>&1)
echo "$install_out" > "$p10dir/install.txt"
settings="$p10dir/ws/.claude/settings.local.json"
if [ ! -f "$settings" ]; then
  say "  P10 INCONCLUSIVE: saga install wrote no settings file ($p10dir/install.txt)"
  results+=("P10 INCONCLUSIVE")
else
  run_claude "$p10dir/cfg" "$p10dir/ws" "$settings" \
    'Run exactly one Bash command: echo saga-probe. Then stop.' \
    "$p10dir/native.jsonl" "$(new_session_id)"
  # stderr stays out of the JSON: doctor exits non-zero on a failed check
  # and its message would otherwise make the report unparsable.
  doctor=$(cd "$p10dir/ws" && "$SAGA" doctor --json 2>"$p10dir/doctor.err")
  echo "$doctor" > "$p10dir/doctor.json"
  hooks_fire=$(printf '%s' "$doctor" | python3 -c "
import json,sys
try: d=json.load(sys.stdin)
except Exception: print('unparsable'); sys.exit(0)
for c in d.get('checks',[]):
    if c.get('id')=='hooks_fire': print(('ok' if c.get('ok') else 'fail')+': '+c.get('detail','')); sys.exit(0)
print('absent')")
  if ! transcript_ok "$p10dir/native.jsonl"; then
    # doctor would read an empty session and report "no hooks fired",
    # which is true and says nothing about whether hooks fire.
    p10="INCONCLUSIVE"
    hooks_fire="no usable transcript: $(why_no_transcript "$p10dir/native.jsonl")"
  else
    case "$hooks_fire" in
      ok:*) p10="PASS" ;;
      fail:*) p10="FAIL" ;;
      *) p10="INCONCLUSIVE" ;;
    esac
  fi
  say "  P10 hooks_fire $p10: $hooks_fire"
  # uninstall --dry-run must list exactly the events install registered.
  dry=$(cd "$p10dir/ws" && "$SAGA" uninstall --harness claude-code --dry-run 2>&1)
  echo "$dry" > "$p10dir/uninstall-dry-run.txt"
  want=$(python3 - "$settings" <<'PY'
import json, sys
s = json.load(open(sys.argv[1]))
out = []
for ev, entries in (s.get("hooks") or {}).items():
    for e in entries:
        for h in e.get("hooks") or []:
            if "saga" in (h.get("command") or ""):
                out.append("hook %s %s" % (ev, h["command"]))
print("\n".join(sorted(out)))
PY
)
  got=$(printf '%s' "$dry" | grep '^hook ' | sort)
  if [ "$want" = "$got" ] && [ -n "$want" ]; then
    say "  P10 uninstall --dry-run PASS: listing matches what install wrote ($(printf '%s' "$want" | wc -l | tr -d ' ') entries)"
    results+=("P10 $p10")
  else
    say "  P10 uninstall --dry-run FAIL: listing differs from install (see $p10dir/uninstall-dry-run.txt)"
    results+=("P10 FAIL")
  fi
  say "  evidence: $p10dir/doctor.json, $p10dir/native.jsonl"
fi

# --------------------------------------------------------------- P17
# One real PostToolUseFailure payload, for comparison with C34.
say ""
say "P17: capture a live PostToolUseFailure payload"
p17dir="$OUT/p17"; mkdir -p "$p17dir/ws" "$p17dir/cfg"
(cd "$p17dir/ws" && git init -q && git commit -q --allow-empty -m base 2>/dev/null)
cap="$p17dir/capture.sh"
cat > "$cap" <<EOF
#!/bin/bash
cat > "$OUT/posttoolusefailure.json"
exit 0
EOF
chmod +x "$cap"
python3 - "$p17dir/settings.json" "$cap" <<'PY'
import json, sys
json.dump({"hooks": {"PostToolUseFailure": [{"hooks": [{"type": "command", "command": sys.argv[2], "timeout": 30}]}]}},
          open(sys.argv[1], "w"), indent=2)
PY
STUB_TOOL_EXIT=${STUB_TOOL_EXIT:-3} run_claude "$p17dir/cfg" "$p17dir/ws" "$p17dir/settings.json" \
  'Run exactly one Bash command: exit 3. Then stop.' \
  "$p17dir/native.jsonl" "$(new_session_id)"
if ! transcript_ok "$p17dir/native.jsonl"; then
  say "  P17 INCONCLUSIVE: the harness produced no usable transcript: $(why_no_transcript "$p17dir/native.jsonl")"
  results+=("P17 INCONCLUSIVE")
elif [ -s "$OUT/posttoolusefailure.json" ]; then
  fields=$(python3 -c "
import json,sys
try: d=json.load(open('$OUT/posttoolusefailure.json'))
except Exception: print(''); sys.exit(0)
print(','.join(sorted(d)))")
  say "  P17 PASS: payload captured, fields: $fields"
  say "  compare with harness-facts C34; the fixture replacement is a separate commit"
  results+=("P17 PASS")
else
  say "  P17 INCONCLUSIVE: no PostToolUseFailure payload captured (the event may not fire for this shape)"
  results+=("P17 INCONCLUSIVE")
fi
say "  evidence: $OUT/posttoolusefailure.json, $p17dir/native.jsonl"

say ""
say "summary:"
for r in "${results[@]}"; do say "  $r"; done
say "evidence root: $OUT"
case " ${results[*]} " in
  *FAIL*) exit 1 ;;
  *INCONCLUSIVE*) exit 2 ;;
esac
exit 0
