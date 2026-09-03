# Harness probes: Claude Code 2.1.259

*v0.1, 2026-09-03. Empirical follow-up to `harness-facts.md` §6 ("what stays a probe after this sprint"), plus re-checks of the four facts the layers lean on hardest (timeout, non-JSON stdout, Stop cap, SessionStart sources). Every probe below was run headlessly against the locally installed `claude` 2.1.259 from a scratch project that is not a git repository. Stdin JSON is quoted from the hook's own log; the model-side view is quoted from the `--output-format json` stream and from the session transcript under `~/.claude/projects/`. Nothing here is inferred from documentation.*

Method common to every probe:

- Scratch project: `<scratch>/hookprobe/` with `.claude/settings.json`, `src/notes.txt` (three lines: `alpha line one`, `beta line two`, `gamma line three`) and `src/app.py`. No `git init`.
- Logger hook `hook.sh <label> [stdout-file]`: appends its full stdin to `log/<label>.jsonl`, then prints the named file (or nothing) on stdout and exits 0.
- Driver: `cd <scratch>/hookprobe && claude -p "<prompt>" --output-format json --permission-mode acceptEdits --max-turns N --model haiku --strict-mcp-config --setting-sources project < /dev/null > log/runX.json`. `--setting-sources project` keeps the user's own hooks out; `--strict-mcp-config` keeps MCP servers out. The main model is Haiku 4.5 in every run; total spend for the sprint was under $0.30.
- Transcript: `~/.claude/projects/<escaped scratch path>/<session_id>.jsonl`; sub-agents under `<session_id>/subagents/agent-<id>.jsonl`.

Status vocabulary matches `harness-facts.md`: **verified** (observed), **contradicted** (spec said otherwise; spec edited), **not testable** (could not be triggered headlessly).

## P1 PostToolUse output shapes for `Read`, `Grep`, `Glob`, and `updatedToolOutput` replacement

Closes harness-facts §6 row 1 (C7).

**P1a: capture the shapes.** Hook entry:

```json
{"hooks":{"PostToolUse":[{"matcher":"Read|Grep|Glob","hooks":[{"type":"command","command":"<scratch>/hookprobe/hook.sh post_shape","timeout":10}]}]}}
```

Command: `claude -p "Use the Read tool on src/notes.txt, then the Grep tool with pattern 'beta' and path src, then the Glob tool with pattern '**/*.py'. Then reply with exactly one word: done." ... --max-turns 6`.

Observed first: only `Read` fired. The `system.init` event lists the built-in tools as `["Task","Bash",...,"Read",...,"Write",...]` with **no `Grep` and no `Glob`**; the model used `Bash` `grep -r` and `find` instead. On 2.1.259 in `-p` mode the two tools are not exposed unless named with `--tools`. Re-run for the shapes with `--tools "Read,Grep,Glob,Bash"` (P1d below), after which `init.tools` was `["Bash","Glob","Grep","Read"]`.

Stdin top-level keys on every `PostToolUse` call: `cwd, duration_ms, hook_event_name, permission_mode, prompt_id, session_id, tool_input, tool_name, tool_response, tool_use_id, transcript_path`.

Captured `tool_response` shapes (verbatim, paths shortened):

```json
Read: {"type":"text","file":{"filePath":"<abs>/src/notes.txt","content":"alpha line one\nbeta line two\ngamma line three\n","numLines":4,"startLine":1,"totalLines":4}}
Grep: {"mode":"content","numFiles":0,"filenames":[],"content":"src/notes.txt:2:beta line two","numLines":1,"totalLines":1}
Glob: {"filenames":["src/app.py"],"durationMs":11,"numFiles":1,"truncated":false,"totalMatches":1,"countIsComplete":true}
```

`Grep` in `content` mode reports `numFiles: 0` and `filenames: []`; the file names are only inside `content`. `Read` reports `numLines: 4` for a three-line file with a trailing newline (the empty fourth line is counted and rendered as `4\t`).

**P1b: replace `Read` with the observed shape.** Hook stdout:

```json
{"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedToolOutput":{"type":"text","file":{"filePath":"/x/replaced.txt","content":"NONCE-READ-7Q4Z replaced by hook\n","numLines":1,"startLine":1,"totalLines":1}}}}
```

Prompt: "Use the Read tool on src/notes.txt and then reply with the exact file content you received, verbatim, nothing else." Result: `"NONCE-READ-7Q4Z replaced by hook"` (2 turns). The `tool_result` block the model received was `"1\tNONCE-READ-7Q4Z replaced by hook\n2\t"`, i.e. Claude Code re-rendered the replacement `file.content` with line numbers; the transcript's `toolUseResult` holds the replacement object, not the original. **Verified: replacement works and the original is gone from the model's view.**

**P1c: replace `Read` with a plain string.** Hook stdout `{"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedToolOutput":"NONCE-STRING-33 plain string replacement"}}`. Result: `"alpha line one\nbeta line two\ngamma line three"`. **Verified: a shape mismatch is silently ignored and the original output is used (C7 as documented).** No stderr, no warning in the JSON stream.

**P1e: replace `Grep` and `Glob` with the observed shapes** (script `grepglob.sh` switches on `tool_name`; `--tools "Read,Grep,Glob,Bash"`):

```json
Grep: {"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedToolOutput":{"mode":"content","numFiles":0,"filenames":[],"content":"src/fake.txt:9:NONCE-GREP-55AB replaced","numLines":1,"totalLines":1}}}
Glob: {"hookSpecificOutput":{"hookEventName":"PostToolUse","updatedToolOutput":{"filenames":["src/NONCE-GLOB-77CD.py"],"durationMs":1,"numFiles":1,"truncated":false,"totalMatches":1,"countIsComplete":true}}}
```

Model-visible `tool_result` blocks: `"src/fake.txt:9:NONCE-GREP-55AB replaced"` and `"src/NONCE-GLOB-77CD.py"`. Result text quoted both verbatim. **Verified for all three tools.**

Verdict: `Read`, `Grep` and `Glob` results are replaceable on 2.1.259 with the shapes above. Shape-spec §7.1 row for these tools moves from *probe* to verified, with the fixture round-trip kept as the install-time guard against a shape change in a later release. Grep and Glob are absent from the `-p` tool list unless `--tools` names them, so the install probe must pass `--tools` or the fixture never fires.

## P2 PreToolUse `updatedInput.model` on `Agent`

Closes harness-facts §6 row 2 (C19). Hook entry (matcher `Agent|Task` because `init.tools` names the tool `Task` while hook input names it `Agent`; the input name is what the matcher sees):

```json
{"hooks":{"PreToolUse":[{"matcher":"Agent|Task","hooks":[{"type":"command","command":"<scratch>/hookprobe/agentmodel.sh","timeout":10}]}],
 "PostToolUse":[{"matcher":"Agent|Task","hooks":[{"type":"command","command":"<scratch>/hookprobe/hook.sh p2post","timeout":10}]}],
 "SubagentStart":[{"hooks":[{"type":"command","command":"<scratch>/hookprobe/hook.sh p2substart","timeout":10}]}]}}
```

`agentmodel.sh` returns `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":(tool_input + {"model":"sonnet"})}}` via `jq`. Main model Haiku. Prompt: "Use the Agent tool with subagent_type general-purpose and the prompt: Reply with exactly the word pong. Do not pass a model field. Then reply with the sub-agent's answer." `--max-turns 4`.

Observed:

- PreToolUse stdin `tool_name` is `"Agent"`; `tool_input` = `{"description":"Test agent responding with pong","prompt":"Reply with exactly the word pong.","subagent_type":"general-purpose"}` (no `model`).
- PostToolUse stdin `tool_input` now carries `"model":"sonnet"` (the hook's rewrite is what PostToolUse sees) and `tool_response.resolvedModel` = `"claude-sonnet-5"`.
- `result.modelUsage` keys: `["claude-haiku-4-5-20251001","claude-sonnet-5"]`.
- Sub-agent transcript `<session>/subagents/agent-ae24324061b858f69.jsonl`: assistant `message.model` = `"claude-sonnet-5"`, text `"pong"`.
- `SubagentStart` stdin: `{"hook_event_name":"SubagentStart","agent_type":"general-purpose","agent_id":"ae24324061b858f69"}` plus common fields.

Two side findings. (1) In `-p` mode on 2.1.259 the `Agent` tool ran **asynchronously**: the tool result was "Async agent launched successfully ... The agent is working in the background", the JSON stream emitted two `result` events (first `"The agent is running in the background..."`, then `"pong"`), and `PostToolUse.tool_response` had keys `agentId, canReadOutputFile, description, isAsync, outputFile, prompt, resolvedModel, status` and **no `usage`**. The documented `usage` block (C19) is therefore only present for a synchronous completion; trace must not rely on it and should read the sub-agent transcript instead. (2) `resolvedModel` is present on the async shape, so route's check (route-spec §6.1) still works.

Verdict: **verified.** A hook-written `updatedInput.model` is honoured; `resolvedModel`, `modelUsage` and the sub-agent transcript all agree.

## P3 PreToolUse `additionalContext` reaches the model

Re-check of C4 and C5 with a nonce. Hook entry: `PreToolUse` matcher `Read`, stdout `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","additionalContext":"The secret nonce is NONCE-CTX-91XK."}}`. Prompt: "Use the Read tool on src/notes.txt. Then reply with any token starting with NONCE- that you can see anywhere in your context, verbatim, or the word none."

Result: `"NONCE-CTX-91XK"` (2 turns). The transcript records two attachments after the tool call, `{"type":"hook_success","hookName":"PreToolUse:Read",...}` and `{"type":"hook_additional_context","content":["The secret nonce is NONCE-CTX-91XK."],"hookName":"PreToolUse:Read","hookEvent":"PreToolUse"}`, and the `tool_result` block itself is the unmodified file. So the context rides beside the tool result as a separate attachment, not inside it. **Verified.**

## P4 PreToolUse hook past its timeout

Re-check of C23. Hook entry: `PreToolUse` matcher `Read`, `"timeout": 3`, script `slow.sh` logs a timestamp, `sleep 8`, logs a second timestamp, then prints a `deny`. A `PostToolUse` logger on `Read` records whether the tool ran.

Observed: `log/p4.times` holds **one** timestamp (the process was killed at the timeout; the second `date` never ran, the late `deny` was never emitted). `PostToolUse` fired once; the `tool_result` was the real file; result `"alpha line one"`. Nothing on stderr and no error event in the JSON stream. **Verified: fails open, silently.** Contracts §1.1's internal deadline (`hook.deadline_ms`) stands as the only defence.

## P5 Non-JSON stdout from a PreToolUse hook

Re-check of C25. Hook entry: `PreToolUse` matcher `Read`, script prints `this is not json NONCE-PLAIN-12` and exits 0; `PostToolUse` logger on `Read`.

Observed: `PostToolUse` fired (the read proceeded); result: first line `alpha line one`, NONCE tokens seen: `none`. Nothing on stderr. **Verified: plain-text stdout on exit 0 allows, and the text is not shown to the model** (matches C4's "debug log only" sentence for plain stdout on PreToolUse).

## P6 Stop hook block cap

Re-check of C10 and C11. Hook entry: `Stop` (no matcher), script counts its own invocations in `log/p6.jsonl` and always returns `{"decision":"block","reason":"NONCE-STOP block N: reply with exactly the word pongN"}`. Prompt: "Reply with exactly one word: ping." `--max-turns 30`.

Observed: the Stop hook fired **9** times; `stop_hook_active` was `false` on the first call and `true` on the next eight. Assistant texts in order: `ping, pong1, pong2, pong3, pong4, pong5, pong6, pong7, pong8`. Blocks 1 to 8 were honoured; block 9 was recorded in the transcript as a `hook_blocking_error` attachment and a user message `"Stop hook feedback:\nNONCE-STOP block 9: ..."` but the model was not called again and the session ended (`num_turns: 10`, `result: ""`). No "cap reached" message appears on stderr or in the JSON stream; the ninth block is simply dropped. **Verified: cap is 8 consecutive blocks.** Side finding: after the cap the `-p` `result` field is empty, not the last assistant text, so a wrapper reading `result` sees nothing; `last_assistant_message` on the Stop input and the transcript still carry `pong8`.

## P7 SessionStart `source` values reachable headlessly

Re-check of C13 and C31. Hook entry: `SessionStart` (no matcher) logger. Three commands with one session id:

1. `claude -p "Reply with exactly one word: one." --session-id <uuid> ...` : stdin `{"source":"startup", ...}`, keys `cwd, hook_event_name, session_id, source, transcript_path`.
2. `claude -p "Reply with exactly one word: two." --resume <uuid> ...` : `{"source":"resume", ...}`, keys add `context_tokens, estimated_cache_write_usd, prompt_cache_likely_expired, seconds_since_last_response`.
3. `claude -p "/compact" --resume <uuid> ...` : two hook calls, `{"source":"resume", ...}` then `{"source":"compact", ...}` with keys `cwd, hook_event_name, model, prompt_id, session_id, source, transcript_path`; the run returned `num_turns: 0`, `result: ""`.

**Verified: `startup`, `resume` and `compact` are all reachable headlessly**; `/compact` works as a `-p` prompt on a resumed session. `clear` and `fork` were not exercised. The extra resume-time fields (`context_tokens`, `prompt_cache_likely_expired`, `seconds_since_last_response`, `estimated_cache_write_usd`) are undocumented in the hooks reference and useful to trace's checkpoint logic; treat them as observed, not contractual.

## P8 Transcript JSONL usage fields

Closes harness-facts §6 row 3 (C29). Read from `<session>.jsonl`, `type: "assistant"` lines, `message.usage` (P1a and P7 sessions, Haiku 4.5, 1h cache TTL in effect):

```json
{"input_tokens":10,"cache_creation_input_tokens":7079,"cache_read_input_tokens":13782,"output_tokens":606,
 "output_tokens_details":{"thinking_tokens":327},
 "server_tool_use":{"web_search_requests":0,"web_fetch_requests":0},
 "service_tier":"standard",
 "cache_creation":{"ephemeral_1h_input_tokens":7079,"ephemeral_5m_input_tokens":0},
 "inference_geo":"not_available",
 "iterations":[{"input_tokens":10,"output_tokens":606,"cache_read_input_tokens":13782,"cache_creation_input_tokens":7079,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":7079},"type":"message"}],
 "speed":"standard"}
```

Every assistant line also carries `message.model`. **Verified: the transcript keeps the nested `cache_creation` object with the 5m/1h split**, plus `output_tokens_details.thinking_tokens` and a per-iteration array. Trace-spec §9.3's split is available from the transcript on Claude Code; `split_reason` is only needed for harnesses that flatten it. Caveat: the same assistant message can appear on more than one transcript line (streaming re-writes), so a normaliser must dedupe on `message.id` before summing.

## Summary table

| Probe | Question | Verdict | Fact rows |
|---|---|---|---|
| P1 | `Read`/`Grep`/`Glob` `tool_response` shapes; `updatedToolOutput` honoured | verified; shapes recorded; string mismatch silently ignored; `Grep`/`Glob` need `--tools` in `-p` | C7 |
| P2 | hook-written `updatedInput.model` on `Agent` honoured | verified via `resolvedModel`, `modelUsage`, sub-agent transcript; `Agent` runs async in `-p`, no `usage` in `tool_response` | C19 |
| P3 | PreToolUse `additionalContext` visible to the model | verified; lands as a `hook_additional_context` attachment beside the tool result | C4, C5 |
| P4 | PreToolUse hook past timeout | verified fails open; hook killed at timeout; no error surfaced | C23 |
| P5 | non-JSON stdout, exit 0 | verified allows; text not shown to the model | C25 |
| P6 | Stop block cap | verified 8; ninth block dropped silently; `-p` `result` empty after the cap | C10, C11 |
| P7 | SessionStart `source` | verified `startup`, `resume`, `compact`; `/compact` works under `-p --resume`; undocumented resume fields | C13, C31 |
| P8 | transcript usage 5m/1h split | verified nested `cache_creation` object; dedupe on `message.id` | C29 |

Claude Code 2.1.259, macOS, 2026-09-03. Scratch project and raw logs: `<session scratchpad>/hookprobe/` (`log/run*.json`, `log/p*.jsonl`).
