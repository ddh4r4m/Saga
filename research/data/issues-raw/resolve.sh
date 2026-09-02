#!/bin/bash
# usage: resolve.sh owner/repo num [num...]  -> prints issue meta + last 3 comments (truncated), saves json
RAW=/private/tmp/claude-501/-Users-dharamdhurandhar-Developer-OpenSource/11a03028-5336-4202-92bd-b3ef4e3079f2/scratchpad/issues-raw
mkdir -p $RAW/detail
repo=$1; shift
slug=$(echo $repo | tr '/' '_')
for n in "$@"; do
  f=$RAW/detail/${slug}_${n}.json
  gh api repos/$repo/issues/$n > $f 2>/dev/null || { echo "## $repo#$n FETCH FAILED"; continue; }
  cnt=$(jq .comments $f)
  page=$(( (cnt + 99) / 100 )); [ $page -lt 1 ] && page=1
  gh api "repos/$repo/issues/$n/comments?per_page=100&page=$page" > ${f%.json}.comments.json 2>/dev/null
  echo "## $repo#$n | $(jq -r '"\(.state)/\(.state_reason) r=\(.reactions.total_count) c=\(.comments) created=\(.created_at[0:10]) closed=\((.closed_at//"")[0:10]) closed_by=\(.closed_by.login//"") labels=\([.labels[].name]|join(","))"' $f)"
  echo "TITLE: $(jq -r .title $f)"
  echo "BODY: $(jq -r '.body//""' $f | tr '\n' ' ' | cut -c1-500)"
  jq -r '.[-3:][]|"  -- \(.user.login) \(.created_at[0:10]): \(.body|gsub("\n";" ")|.[0:400])"' ${f%.json}.comments.json
  echo
done
