#!/bin/bash
# Count issues per theme per repo using GitHub search totals. Output TSV: repo, theme, total, open, top3 issue numbers
RAW=/private/tmp/claude-501/-Users-dharamdhurandhar-Developer-OpenSource/11a03028-5336-4202-92bd-b3ef4e3079f2/scratchpad/issues-raw
cd $RAW
REPOS="anthropics/claude-code openai/codex google-gemini/gemini-cli anomalyco/opencode Aider-AI/aider OpenHands/OpenHands cline/cline RooCodeInc/Roo-Code Kilo-Org/kilocode aaif-goose/goose anthropics/claude-agent-sdk-python microsoft/vscode-copilot-chat"
# theme|query  (query is url-encoded manually: spaces as +, quotes as %22)
THEMES='compaction|compact+OR+compaction+OR+%22context+window%22+OR+%22lost+context%22+OR+%22loses+context%22
ignores_instructions|%22CLAUDE.md%22+OR+%22AGENTS.md%22+OR+%22GEMINI.md%22+OR+%22ignores+instructions%22+OR+%22ignoring+instructions%22+OR+%22.clinerules%22
permissions|permission+OR+%22dangerously-skip%22+OR+%22auto-approve%22+OR+%22yolo%22+OR+%22approval%22
cost_tokens|%22token+usage%22+OR+%22tokens+per%22+OR+%22burn%22+OR+%22expensive%22+OR+%22cache+hit%22+OR+%22cache+miss%22+OR+%22cost%22
edit_failures|%22string+not+found%22+OR+%22edit+failed%22+OR+%22patch+failed%22+OR+%22apply_patch%22+OR+%22diff+edit%22+OR+%22old_string%22+OR+%22search/replace%22+OR+%22SEARCH/REPLACE%22
looping|%22infinite+loop%22+OR+%22stuck+in+a+loop%22+OR+%22loops%22+OR+%22keeps+repeating%22+OR+%22same+tool+call%22
model_quality|%22dumber%22+OR+%22degraded%22+OR+%22quality+regression%22+OR+%22got+worse%22+OR+%22quantized%22+OR+%22lazy%22+OR+%22worse+than%22
memory_sessions|%22memory%22+OR+%22remember%22+OR+%22across+sessions%22+OR+%22persist%22
mcp|MCP
multi_agent|subagent+OR+%22sub-agent%22+OR+%22sub+agent%22+OR+swarm+OR+%22parallel+agents%22+OR+orchestrat
sandbox|sandbox+OR+seatbelt+OR+landlock+OR+bubblewrap+OR+%22sandboxed%22
windows|Windows+OR+WSL+OR+PowerShell+OR+%22path+separator%22
rate_limits|%22rate+limit%22+OR+429+OR+%22usage+limit%22+OR+quota+OR+%22limit+reached%22+OR+overloaded
test_gaming|%22fake+tests%22+OR+%22skips+tests%22+OR+%22deletes+tests%22+OR+%22hardcode%22+OR+%22cheat%22+OR+%22gaming%22+OR+%22mocks+instead%22+OR+%22claims+success%22+OR+%22lies%22
hallucination|hallucinat+OR+%22made+up%22+OR+%22invented%22+OR+%22nonexistent+file%22+OR+%22does+not+exist%22
hang_crash|hang+OR+hangs+OR+freeze+OR+frozen+OR+crash+OR+unresponsive
hooks|hook+OR+hooks
git_dataloss|%22data+loss%22+OR+%22deleted+my%22+OR+%22git+reset%22+OR+%22rm+-rf%22+OR+%22destroyed%22+OR+%22wiped%22
language_specific|Python+OR+TypeScript+OR+Rust+OR+Go+OR+Java+OR+Swift+OR+Kotlin+OR+Flutter
inconsistency|%22worked+yesterday%22+OR+%22used+to+work%22+OR+%22inconsistent%22+OR+%22non-deterministic%22+OR+%22nondeterministic%22+OR+%22sometimes%22+OR+%22randomly%22'
OUT=themes.tsv
echo -e "repo\ttheme\ttotal\topen\ttop" > $OUT
echo "$THEMES" | while IFS='|' read theme query; do
  for repo in $REPOS; do
    for i in 1 2 3; do
      res=$(gh api "search/issues?q=repo:$repo+is:issue+$query&sort=reactions&order=desc&per_page=5" --jq '[.total_count, ([.items[]|"#\(.number)(\(.reactions.total_count),\(.state)) \(.title|.[0:70])"]|join(" || "))]|@tsv' 2>/dev/null) && break
      sleep 25
    done
    sleep 2.2
    for i in 1 2 3; do
      op=$(gh api "search/issues?q=repo:$repo+is:issue+is:open+$query&per_page=1" --jq '.total_count' 2>/dev/null) && break
      sleep 25
    done
    sleep 2.2
    echo -e "$repo\t$theme\t$res\t$op" | awk -F'\t' 'BEGIN{OFS="\t"}{print $1,$2,$3,$5,$4}' >> $OUT
  done
  echo "theme $theme done $(date +%T)"
done
echo "THEMES DONE $(date +%T)"
