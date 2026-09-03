import pathlib, re, sys
T = pathlib.Path('/Users/dharamdhurandhar/Developer/OpenSource/Saga/bench/tasks')
for d in sorted(T.iterdir()):
    if not d.is_dir() or not (d/'task.toml').exists(): continue
    c = (d/'CANARY').read_text().strip()
    prompt = (d/'prompt.md').read_text()
    problems = []
    files = [p for p in d.rglob('*') if p.is_file() and 'repo/' not in str(p.relative_to(d)) + '/' and p.name != 'CANARY' and p.name != 'prompt.md']
    for p in files:
        try: s = p.read_text()
        except UnicodeDecodeError: continue
        if c not in s: problems.append(f'canary missing: {p.relative_to(d)}')
    if c in prompt: problems.append('canary in prompt')
    gold = d/'controls'/'gold.patch'
    if gold.exists():
        for line in gold.read_text().splitlines():
            if line.startswith('+') and not line.startswith('+++'):
                body = line[1:].strip()
                if len(re.sub(r'\s', '', body)) >= 20 and body in prompt: problems.append(f'gold line in prompt: {body[:40]}')
    for p in (d/'oracle').rglob('*'):
        if p.is_file() and p.name in prompt and p.name not in ('run.sh',): problems.append(f'oracle name in prompt: {p.name}')
    src = sum(len(p.read_text().splitlines()) for p in (d/'repo').rglob('*') if p.is_file() and p.suffix in ('.py','.ts','.mjs','.json','.toml','.md','.yml') and 'fixtures' not in str(p) and 'samples' not in p.name)
    print(f"{d.name}: canary={c} source_lines={src} {'OK' if not problems else problems}")
