import pathlib, re, subprocess, sys
T = pathlib.Path('/Users/dharamdhurandhar/Developer/OpenSource/Saga/bench/tasks')
V = pathlib.Path(sys.argv[1])
for d in sorted(T.iterdir()):
    if not d.is_dir() or not (d/'task.toml').exists(): continue
    gates = re.findall(r'^- \[ \] (\w+):.*\n((?:    .*\n)+)', (d/'contract.md').read_text(), flags=re.M)
    for state in ('baseline', 'gold'):
        ws = V/d.name/state
        if not ws.exists(): continue
        res = []
        for gid, body in gates:
            chk = re.search(r'^    CHECK: (.*)$', body, flags=re.M).group(1)
            exp = re.search(r'^    EXPECT: (.*)$', body, flags=re.M).group(1)
            r = subprocess.run(['bash', '-c', chk], cwd=ws, capture_output=True, text=True)
            out = r.stdout + r.stderr
            m = re.fullmatch(r'/(.*)/([a-z]*)', exp)
            ok = bool(re.search(m.group(1), out, flags=(re.M if 'm' in m.group(2) else 0) | (re.S if 's' in m.group(2) else 0))) if m else (exp in out)
            res.append(f"{gid}={'met' if ok else 'unmet'}")
        print(f"{d.name} {state}: {' '.join(res)}")
