# Brief: saga guard as the identical deny-only safety hook in both arms (docs/12 row 6)

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Two commits: part 1 (the hook entry and its tests), part 2 (bench wiring, disclosure, blocking test, docs). Protocol amendment for the arm table: docs/12 §13, 2026-09-06 (written by saga).

## 1. Why

Row 5 takes the worktree substitute for containers, so row 6 is a must: the agent runs on the owner's machine, and a destructive command (guard-spec D1 to D11: `rm -rf` of home or repo root, `git push --force`, credential exfiltration, and the rest) must be denied in both arms by the same code, so the safety net is not a treatment. The classifier exists (`internal/guard`, `saga guard check-cmd`, 18 incident fixtures and 40 controls, zero escapes) but no hook runs it.

## 2. Decisions (taken)

1. A new entry `saga guard hook claude-code PreToolUse` (separate from the composed `saga hook` chain, which stays as it is): reads the PreToolUse payload, acts only when `tool_name` is `Bash` (or `PowerShell`), runs `guard.Check` with `DefaultPolicy` in deny-only mode (hard-deny rules D1 to D11 only; an `ask` verdict becomes allow; no snapshot, no `.saga/` read or write, no policy file lookup), and renders the documented JSON: `permissionDecision: deny` with `permissionDecisionReason` naming the rule id and the segment on a deny, `{}` otherwise, exit 0 in both cases. An internal error (parse failure, panic) allows and logs `error`; it never blocks work by accident, because a false deny in one arm would be a treatment.
2. Every decision is one JSON line appended to the file named by `SAGA_GUARD_LOG` (set by the bench in the hook's environment through the settings `env` block or the command string; pick the one the harness passes to hooks and say which): `ts`, `session_id`, `tool_use_id`, `verdict`, `rules`, `sha256` of the command, never the command text.
3. The bench registers exactly this one hook, with an identical command string, in both arms. Arm A's `settings:no-hooks` block becomes `settings:no-saga-hooks-but-safety`; the disclosure's `hooks` lists it in both arms with `role: "safety"` and, in arm A, `deviation_from_bare: true` and one sentence of reason. `run.json` gains `guard_denies` (integer, from the log; schema required) and the report prints the per-arm total next to `blocked_reach_attempts` (expected 0 in both arms on this corpus, as on the controls C-01 to C-40).
4. The §10.2 blocking test extends: arm A's settings carry exactly the safety hook and nothing else; the fake agent's `rm -rf` of the workspace root is denied in both arms and logged; the sentinel and the shim still hold.

## 3. Tests

1. Hook path over every row of `fixtures/guard/incidents.toml`: each I row denies with its rule ids, each C row allows, through the hook entry with a real PreToolUse payload, not only through `check-cmd`.
2. Log line shape; no command text in the log; the log file is the only file the hook writes (assert nothing under the workspace changed, `.saga` absent in arm A).
3. Fail-open on a malformed payload and on a shell the parser does not implement, with an `error` log line.
4. Blocking test per decision 4; Prepare disclosure per decision 3; `guard_denies` per run in a replay archive.
5. Timing: the hook on the 40 controls stays under 50 ms p95 on this machine (report the number; harness-facts C23 is the 60 s hook timeout, so this is about not adding wall time, not about correctness).

## 4. Docs, same commits

`docs/specs/guard-spec.md` §8: the safety hook entry, deny-only, the log, and the sentence that the composed chain's guard step (snapshots, ask, masking) remains unwired; `docs/specs/bench-spec.md` §4.2 hooks row: the safety-hook exception, identical in both arms, disclosed; `docs/specs/IMPLEMENTATION-STATUS.md`; docs/12 §10 row 6 closed with the hash (the one docs/12 edit granted; §4's arm table is amended by saga).

## 5. Out of scope

Snapshots, masking, `ask` decisions, policy files, the composed chain's guard step, any change to the D rules or the fixtures.

## 6. Report

Hashes; the fixture-suite result through the hook path (escapes, control denies); the p95; the blocking test's new assertions; anything left out. Under 30 lines.
