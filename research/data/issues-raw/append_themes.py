import csv,re,io
R='/private/tmp/claude-501/-Users-dharamdhurandhar-Developer-OpenSource/11a03028-5336-4202-92bd-b3ef4e3079f2/scratchpad/'
rows=list(csv.reader(open(R+'issues-raw/themes2.tsv'),delimiter='\t'))[1:]
rows=[r for r in rows if len(r)>=4 and r[2].isdigit()]
themes=[];repos=[]
for r in rows:
    if r[1] not in themes: themes.append(r[1])
    if r[0] not in repos: repos.append(r[0])
short={'anthropics/claude-code':'claude-code','openai/codex':'codex','google-gemini/gemini-cli':'gemini-cli','anomalyco/opencode':'opencode','cline/cline':'cline'}
out=io.StringIO()
out.write('\n### 3a. Theme counts (title-only search; total / still open)\n\n')
out.write('| Theme | '+' | '.join(short[x] for x in repos)+' |\n|---|'+'---|'*len(repos)+'\n')
d={(r[0],r[1]):(r[2],r[3]) for r in rows}
for t in themes:
    out.write('| '+t+' | '+' | '.join(f"{d[(x,t)][0]} / {d[(x,t)][1]}" if (x,t) in d else '-' for x in repos)+' |\n')
out.write('\nTop-3 by reactions per theme (from the same search), abbreviated:\n\n')
for r in rows:
    top=r[4] if len(r)>4 else ''
    top=re.sub(r'\s+',' ',top)[:230]
    out.write(f"- **{r[1]}** / {short[r[0]]}: {top}\n")
s=open(R+'research-issues.md').read()
new=out.getvalue()
if '<!--THEMES-->' in s:
    s=re.sub(r'<!--THEMES-->.*?<!--/THEMES-->', '<!--THEMES-->'+new+'<!--/THEMES-->', s, flags=re.S)
else:
    s=s.replace('## 4. Long-lived open issues', '<!--THEMES-->'+new+'<!--/THEMES-->\n\n## 4. Long-lived open issues',1)
open(R+'research-issues.md','w').write(s)
print('themes appended:',len(themes),'themes')
