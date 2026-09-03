# retry-kit

`retry(fn, { attempts, baseMs })` retries a failing async call. Wait before retry i is `baseMs * 2^i * (1 + r)`, r in [0, 1). `npm test`.
