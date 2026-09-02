#!/bin/bash
# Title-only theme counts (<=5 OR operators each). Output TSV: repo theme total open top3
RAW=/private/tmp/claude-501/-Users-dharamdhurandhar-Developer-OpenSource/11a03028-5336-4202-92bd-b3ef4e3079f2/scratchpad/issues-raw
cd $RAW
REPOS="anthropics/claude-code openai/codex google-gemini/gemini-cli anomalyco/opencode cline/cline"
THEMES='compaction|compact+OR+compaction+OR+%22context+window%22+OR+%22context+loss%22+OR+%22loses+context%22
instructions|%22CLAUDE.md%22+OR+%22AGENTS.md%22+OR+%22GEMINI.md%22+OR+%22ignores+instructions%22+OR+%22ignoring+instructions%22
permissions|permission+OR+permissions+OR+%22auto-approve%22+OR+dangerously+OR+%22approval%22
cost_tokens|%22token+usage%22+OR+%22usage+limit%22+OR+burning+OR+%22cache%22+OR+cost
edit_failures|%22string+not+found%22+OR+%22edit+failed%22+OR+%22patch+failed%22+OR+apply_patch+OR+%22diff+edit%22
looping|loop+OR+loops+OR+stuck+OR+infinite+OR+repeating
model_quality|dumber+OR+degraded+OR+regression+OR+worse+OR+lazy
memory|memory+OR+%22across+sessions%22+OR+remember+OR+persistent+OR+forget
mcp|MCP
multi_agent|subagent+OR+subagents+OR+%22sub-agent%22+OR+%22agent+teams%22+OR+swarm
rate_limits|%22rate+limit%22+OR+429+OR+overloaded+OR+%22limit+reached%22+OR+quota
hang_crash|hang+OR+hangs+OR+freeze+OR+crash+OR+unresponsive
windows|Windows+OR+WSL+OR+PowerShell
hallucination|hallucinat+OR+%22made+up%22+OR+%22does+not+exist%22+OR+fabricat+OR+%22claims%22
test_gaming|fake+OR+%22skips+tests%22+OR+%22deletes+tests%22+OR+cheat+OR+hardcode
sandbox|sandbox+OR+seatbelt+OR+landlock+OR+bubblewrap+OR+sandboxed
hooks|hook+OR+hooks
dataloss|%22data+loss%22+OR+deleted+OR+%22rm+-rf%22+OR+wiped+OR+destroyed
inconsistency|%22worked+yesterday%22+OR+%22used+to+work%22+OR+inconsistent+OR+randomly+OR+sometimes'
OUT=themes2.tsv
echo -e "repo\ttheme\ttotal\topen\ttop" > $OUT
echo "$THEMES" | while IFS='|' read theme query; do
  for repo in $REPOS; do
    res=""; op=""
    for i in 1 2 3; do
      res=$(gh api "search/issues?q=repo:$repo+is:issue+in:title+$query&sort=reactions&order=desc&per_page=3" --jq '[.total_count, ([.items[]|"#\(.number)(\(.reactions.total_count),\(.state)) \(.title|.[0:80])"]|join(" || "))]|@tsv' 2>/dev/null) && break
      sleep 25
    done
    sleep 2.2
    for i in 1 2 3; do
      op=$(gh api "search/issues?q=repo:$repo+is:issue+is:open+in:title+$query&per_page=1" --jq '.total_count' 2>/dev/null) && break
      sleep 25
    done
    sleep 2.2
    echo -e "$repo\t$theme\t$res\t$op" | awk -F'\t' 'BEGIN{OFS="\t"}{print $1,$2,$3,$5,$4}' >> $OUT
  done
  echo "theme $theme done $(date +%T)"
done
echo "THEMES2 DONE $(date +%T)"
