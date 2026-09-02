#!/bin/bash
RAW=/private/tmp/claude-501/-Users-dharamdhurandhar-Developer-OpenSource/11a03028-5336-4202-92bd-b3ef4e3079f2/scratchpad/issues-raw
cd $RAW
REPOS="anthropics/claude-code openai/codex google-gemini/gemini-cli anomalyco/opencode opencode-ai/opencode Aider-AI/aider OpenHands/OpenHands cline/cline RooCodeInc/Roo-Code Kilo-Org/kilocode aaif-goose/goose anthropics/claude-agent-sdk-python microsoft/vscode-copilot-chat obra/superpowers mattpocock/skills Leonxlnx/unlazy anthropics/skills vercel-labs/skills github/spec-kit bmad-code-org/BMAD-METHOD gsd-build/get-shit-done ruvnet/ruflo oraios/serena upstash/context7 mem0ai/mem0 letta-ai/letta getzep/graphiti modelcontextprotocol/servers hesreallyhim/awesome-claude-code JuliusBrussee/caveman DietrichGebert/ponytail wilpel/caveman-compression"
FIELDS='{total:.total_count, items:[.items[]|{n:.number,title:.title,state:.state,state_reason:.state_reason,r:.reactions.total_count,up:.reactions["+1"],c:.comments,created:.created_at,closed:.closed_at,updated:.updated_at,labels:[.labels[].name],author:.user.login,url:.html_url,body:(.body//""|.[0:600])}]}'
for repo in $REPOS; do
  slug=$(echo $repo | tr '/' '_')
  echo "=== $repo $(date +%T)"
  [ -s ${slug}.repo.json ] || gh api repos/$repo > ${slug}.repo.json 2>/dev/null
  [ -s ${slug}.releases.json ] || gh api "repos/$repo/releases?per_page=100" --jq '[.[]|{tag:.tag_name,published:.published_at}]' > ${slug}.releases.json 2>/dev/null
  for q in "is:open|open_reactions|reactions" "is:closed|closed_reactions|reactions" "is:open|open_comments|comments" "is:closed|closed_comments|comments"; do
    IFS='|' read st name sort <<< "$q"
    for i in 1 2 3; do
      gh api "search/issues?q=repo:$repo+is:issue+$st&sort=$sort&order=desc&per_page=30" --jq "$FIELDS" > ${slug}.${name}.json 2>${slug}.${name}.err && break
      echo "retry $repo $name"; sleep 20
    done
    sleep 2.3
  done
done
echo "DONE $(date +%T)"
