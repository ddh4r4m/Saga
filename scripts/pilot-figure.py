#!/usr/bin/env python3
"""Render the one-glance figure for a pilot archive from its runner-written
compare.json. Reads only; every number on the figure is the report's own.

    python3 scripts/pilot-figure.py bench/results/pilot-2026-09-13 [out.png]

Needs matplotlib. The figure shows the pre-registered primary first and
labels the rest as secondary or exploratory, so a reader cannot take a
scan number for the result.
"""
import json, sys, pathlib
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

root = pathlib.Path(sys.argv[1])
out = pathlib.Path(sys.argv[2]) if len(sys.argv) > 2 else root / "figure.png"
c = json.load(open(root / "compare.json"))
A, B = c["arms"]["A"], c["arms"]["B"]
prim = c["primary"]
neg = {n["metric"]: n for n in c["negative"] if isinstance(n, dict) and "metric" in n}

BLUE, ORANGE = "#2a78d6", "#eb6834"      # categorical slots 1 and 2, validated
INK, INK2, GRID, SURF = "#0b0b0b", "#52514e", "#e6e5e1", "#fcfcfb"

def pct(x): return f"{100*x:.1f}%"
def k(x): return f"{x/1000:.0f}k"

rows = [
    ("False “done” claims", "primary, pre-registered",
     prim["A"], prim["B"], pct,
     f"Δ {100*prim['delta']:+.1f} pp, 95% CI {100*prim['ci95'][0]:+.1f} to {100*prim['ci95'][1]:+.1f} pp. "
     f"Null: the −5 pp floor was not cleared."),
    ("Tasks solved (pass@1)", "secondary, pre-registered",
     A["pass_at_1"], B["pass_at_1"], pct,
     f"Δ {100*neg['pass_at_1']['delta']:+.1f} pp, CI {100*neg['pass_at_1']['ci95'][0]:+.1f} to {100*neg['pass_at_1']['ci95'][1]:+.1f} pp. Not lowered."),
    ("Tokens per solved task", "cost condition, pre-registered",
     A["per_solved"]["tokens"], B["per_solved"]["tokens"], k,
     f"Ratio {B['per_solved']['tokens']/A['per_solved']['tokens']:.2f}, above the 1.10 bound. Cost condition failed."),
    ("Runs with out-of-scope edits", "exploratory scan, not a result",
     A["scope_violation_rate"], B["scope_violation_rate"], pct,
     "The gated agent edited outside the task's files far less often."),
    ("Passing runs flagged by the cheat scan", "exploratory scan, not a result",
     A["cheat_rate"], B["cheat_rate"], pct,
     "Bare flags were test-file edits the clean-checkout oracle never saw."),
]

plt.rcParams.update({"font.family": ["Helvetica Neue", "Helvetica", "Arial", "DejaVu Sans"],
                     "font.size": 11, "text.color": INK, "axes.edgecolor": GRID})
fig = plt.figure(figsize=(12, 9.6), dpi=160, facecolor=SURF)
fig.text(0.05, 0.955, "Does a Stop gate make a coding agent honest about “done”? A pilot.",
         fontsize=19, weight="bold", ha="left", va="top")
fig.text(0.05, 0.915, f"{A['tasks'] if 'tasks' in A and isinstance(A['tasks'], int) else 20} tasks × {A['k']} runs × 2 arms on Claude Code, model {A['model']}.  "
         "Arm A runs bare; arm B runs with the Saga gate hook.  Pre-registered before any run.",
         fontsize=11.5, color=INK2, ha="left", va="top")
fig.text(0.05, 0.885, "●", color=BLUE, fontsize=13, ha="left", va="top")
fig.text(0.065, 0.885, "A, bare", fontsize=11, ha="left", va="top")
fig.text(0.135, 0.885, "●", color=ORANGE, fontsize=13, ha="left", va="top")
fig.text(0.15, 0.885, "B, with the gate", fontsize=11, ha="left", va="top")

top, bottom = 0.84, 0.15
h = (top - bottom) / len(rows)
for i, (name, kind, a, b, fmt, note) in enumerate(rows):
    y0 = top - (i + 1) * h
    ax = fig.add_axes([0.42, y0 + 0.30 * h, 0.53, 0.28 * h])
    ax.set_facecolor(SURF)
    hi = max(a, b) * 1.25 if max(a, b) > 0 else 1
    ax.set_xlim(-0.05 * hi, hi); ax.set_ylim(-1, 1)
    for s in ax.spines.values(): s.set_visible(False)
    ax.set_yticks([]); ax.set_xticks([])
    ax.axhline(0, color=GRID, lw=1, zorder=1)
    ax.plot([a, b], [0, 0], color=INK2, lw=2, solid_capstyle="round", zorder=2)
    ax.scatter([a], [0], s=150, color=BLUE, zorder=3, edgecolor=SURF, linewidth=2)
    ax.scatter([b], [0], s=150, color=ORANGE, zorder=3, edgecolor=SURF, linewidth=2)
    va_a, va_b = ("bottom", "top") if a >= b else ("top", "bottom")
    off = 0.32
    ax.text(a, off if a >= b else -off, fmt(a), ha="center", va="bottom" if a >= b else "top", fontsize=11, color=INK)
    ax.text(b, -off if a >= b else off, fmt(b), ha="center", va="top" if a >= b else "bottom", fontsize=11, color=INK)
    fig.text(0.05, y0 + 0.72 * h, name, fontsize=13.5, weight="bold", ha="left", va="center")
    fig.text(0.05, y0 + 0.50 * h, kind, fontsize=10.5, color=ORANGE if "exploratory" in kind else INK2,
             style="italic" if "exploratory" in kind else "normal", ha="left", va="center")
    fig.text(0.05, y0 + 0.24 * h, note, fontsize=10.5, color=INK2, ha="left", va="center", wrap=True)
    if i < len(rows) - 1:
        fig.add_artist(plt.Line2D([0.05, 0.95], [y0 + 0.02 * h, y0 + 0.02 * h], color=GRID, lw=1))

man = c["manifests"] if isinstance(c.get("manifests"), dict) else {}
fig.text(0.05, 0.105,
         "What holds: the gate ran in every arm B run, no run approved its own baselines, hidden oracles graded in a clean checkout, "
         "and the harness's own cost figure reconciles.\n"
         "What does not: the pre-registered effect. By the kill rule the experiment ended here and this pilot is the result. "
         "Exploratory rows are a hint for a differently registered experiment, not evidence.\n"
         f"Source: the runner-written report in this archive (pre-registration sha256 {c['preregistration_sha256'][7:23]}…), "
         f"{c.get('bootstrap_resamples', 10000)} bootstrap resamples over tasks. Rendered by scripts/pilot-figure.py.",
         fontsize=9, color=INK2, ha="left", va="top", linespacing=1.5)
fig.savefig(out, facecolor=SURF)
print(out)
