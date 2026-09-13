# Brief: `saga bench figure`, the pilot figure in Go with the null drawn

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. Two commits: d1, then d2. Plan by a planning subagent, reviewed and adopted by saga; the plan's own text follows the decisions. Nothing under `bench/results/` other than the generated `figure.svg` (and the removal of `figure.png`) changes; archived JSON is read only.

## Decisions

1. `saga bench figure <archive-dir> [--out figure.svg] [--per-task]` in a new dependency-free package `internal/bench/figure` (deterministic SVG string writer), dispatched in `internal/cli/cmd/bench.go` beside `report` and `compare`. Reads `compare.json` only, through `report.Report` so schema drift fails the build.
2. Same five rows and the same two categorical colours as `scripts/pilot-figure.py` (blue `#2a78d6` arm A, orange `#eb6834` arm B, validated), same kind labels verbatim (`primary, pre-registered`, `secondary, pre-registered`, `cost condition, pre-registered`, `exploratory scan, not a result` in orange italic), same row notes, same footer (what holds, what does not, source with the pre-registration sha and the resample count).
3. New on every pre-registered row: a delta panel, Δ as a dot with its 95 percent interval as a thin line, a hairline at zero, and on the primary row a second hairline at −0.05 labelled `kill floor`. Exploratory rows get no delta panel and the literal note `not tested, no interim-look protection`.
4. Read `primary.A` and `primary.B` for the first row, never `arms.A.false_done` (null by the report defect fixed in cd7ddc5 for future runs, still null in the pilot's archived JSON); pass@1 from the `negative` entry with `metric == "pass_at_1"` (or wherever the report carries its CI; name the field you use); tokens per solved from `per_solved`; scope and cheat rates from `arms.{A,B}`.
5. Output goes to `<archive>/figure.svg`; README's embedded image path changes from `.png` to `.svg` and its caption names the subcommand; `scripts/pilot-figure.py` and `figure.png` are removed in the d1 commit, the Python file's docstring intent moved into the package doc. Fonts: a system sans stack with a generic fallback, sizes fixed in px so the golden file is stable.
6. Tests: golden `internal/bench/figure/testdata/pilot.golden.svg` byte-equal against `bench/results/pilot-2026-09-13/compare.json` (refreshed with `-update`); a structural test that every exploratory row contains `not a result` and no CI line element, and that the primary row contains `kill floor`; a test that the README's image path exists in the tree; a determinism test (two renders, equal bytes).
7. d2, second commit: `--per-task` writes `figure-per-task.svg`: twenty rows, one dumbbell per task of median tokens A vs B from `arms.{A,B}.per_task[]`, sorted by B − A, `c/n` as text right of each row, tasks in `instability.unstable_tasks` marked with a hollow dot, caption literal `descriptive; medians per task; no test computed`. Golden test as d1. Embedded in docs/15 (not the README) with one sentence.

## Stop line

No HTML, no dashboards, no percent-of-baseline bars, no delta table in README prose, no session-health page, no badge. Nothing quoted in README prose beyond what is already there.

## Report

Per commit: hash, test names, and the rendered SVG opened once in a browser to check for label collisions (say what you saw). Under 15 lines each.

---

Appendix: the planning subagent's text, verbatim.

# Saga chart visuals: plan

Ground truth checked: `bench/results/pilot-2026-09-13/compare.json` (schema `saga.bench.report/1`) is the only archive with a `compare.json`; dev/smoke archives carry `compare.md` only. `.saga/trace/sessions/` does not exist in this repo (zero ledgers). The trace layer records `context_tokens` per call but no model window size; `prices/default.toml` `long_context.over = 200000` is a billing threshold, not a window. Dataviz guidance read from `references/choosing-a-form.md`, `anti-patterns.md`, `palette.md` (`SKILL.md` is absent from the bundled dir). Note `figure.png` is currently modified and uncommitted in the working tree.

## 1. Verdicts

**(a) README "Numbers" section, grouped percent-of-baseline bars + delta table: not worth it.** With two arms every group degenerates to one gray bar and one orange bar, which is the "one-bar bar chart" anti-pattern; the shape of screenshot 1 is a positive-result pitch, and docs/12 §13's kill record says "the pilot number is not quoted outside the pilot report", so a delta table in README text is disallowed outright. The README already embeds the archive's own figure, which is the form `choosing-a-form.md` prescribes for before→after per item (dumbbell).

**(b) `saga bench report --html` with stat tiles and per-task charts: not worth it.** GitHub renders neither HTML nor stat tiles; `compare.md` is the report and is checksummed as the runner wrote it; a KPI row of ten null deltas says less than the secondary table already does. The one thing the tables hide (where arm B's extra tokens come from, per task) is better served by a single SVG, see (d2).

**(c) Session-health page from the trace ledger: not worth it now.** There are no sessions to draw, no window size to divide `context_tokens` by, and no "could have saved" counterfactual anywhere in `LedgerRow` or `Report` (`internal/trace/ledger.go`); a page built on assumed per-provider windows would be the kind of unsupported number the project just stopped quoting. Revisit only if `saga trace ledger` gains a window field and the repo accumulates real sessions.

**(d) Worth it: port the pilot figure to Go as `saga bench figure`, adding a delta-with-CI panel.** The current PNG prints the CI as prose; drawing Δ against zero and the −5 pp floor makes the null visible instead of stated, and a Go writer makes the figure regenerable under `go test` like `report.json` (README week-6 exit criterion: byte-identical on regeneration).

## 2. The worth-it item(s)

### d1. `saga bench figure <archive-dir> [--out figure.svg]`

**Fields read (compare.json only, unmarshalled into `report.Report` so schema drift fails the build, not the figure):** `arms.A.model`, `arms.A.tasks`, `arms.A.k`; `primary.{metric,A,B,delta,ci95,supported}` (read `primary.A`, not `arms.A.false_done`, which is null by report defect 1); `secondary[]` where `metric=="pass_at_1"` → `{A,B,delta,ci95}`; `per_solved.{A,B}.tokens`; `arms.{A,B}.scope_violation_rate`; `arms.{A,B}.cheat_rate`; `preregistration_sha256`, `bootstrap_resamples`, `total_cost_usd`, `created`.

**Form.** Five rows as now; each pre-registered row gets a second small panel: Δ as a dot with its 95% CI as a thin line, a hairline at 0, and on the primary row a second hairline at −0.05 labelled "kill floor". `choosing-a-form.md`: before→after per item → dumbbell; delta to a baseline → mark against a baseline line, one axis. Two fixed categorical slots (blue A, orange B per `palette.md`), thin marks, direct labels, no grid beyond hairlines (`anti-patterns.md`: no dashed rules, no number on every point).

**Honesty guardrails, literal in the SVG:** row kind text `primary, pre-registered` / `secondary, pre-registered` / `cost condition, pre-registered` / `exploratory scan, not a result` (orange italic); exploratory rows get **no** Δ/CI panel and the note "not tested, no interim-look protection" (docs/15); primary note "Null: the −5 pp floor was not cleared"; footer carries the pre-registration sha and resample count; title keeps "A pilot". The figure never reads anything outside the archive.

**Where.** New package `internal/bench/figure` (a ~150-line deterministic SVG string writer, `text/template` or `fmt`, no dependencies); dispatch in `internal/cli/cmd/bench.go` beside `report`/`compare`. Output `figure.svg` in the archive dir; README `<img>` path changes from `.png` to `.svg`. `scripts/pilot-figure.py` is deleted, its docstring intent moved to the package doc.

**Tests.** Golden `figure.svg` byte-equal against `bench/results/pilot-2026-09-13/compare.json` (`testdata/pilot.golden.svg`, refreshed by `-update`); structural test asserting each exploratory row contains the literal "not a result" and no `<line class="ci">`; a test that the README's embedded path exists. **Effort: 5 h.**

### d2 (optional). Per-task tokens, same command, `--per-task`

**Fields:** `arms.{A,B}.per_task[].{task,median_tokens,c,n}`, `instability.unstable_tasks[]`. **Form:** 20-row dumbbell, tasks sorted by B−A, `c/n` as tabular text right of each row, unstable tasks marked with a hollow dot. Caption literal: "descriptive; medians per task; no test computed". Shares d1's writer. Golden SVG test. **Effort: 2 h.**

## 3. Order and stop line

Order: d1, then d2 if the reader of docs/15 wants it. Stop after that.

Do not build: any HTML page or dashboard; percent-of-baseline bars or a README delta table; a session-health page or any "share of window"/"could have saved" number; per-run or per-event charts (hook overhead p50/p95 by event is already a table in the report and `choosing-a-form.md` says a handful of numbers is a table, not a chart); `bench badge` output (refused below tier `publish`); anything that puts a pilot number into README text rather than into the archive's own figure.
