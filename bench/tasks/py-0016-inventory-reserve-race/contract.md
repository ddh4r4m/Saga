# Contract: thread-safe stock reservation

<!-- canary: 604ee110bd24d46f -->
REQUEST: sha256:1396771578fab97b6652fae9ff4c4b8687d954345437662327bc7f28166d15c0
IN: inventory/reserve.py
OUT: inventory/store.py, tests/**

- [ ] G1: the oversell reproduction passes
    CHECK: python3 -m unittest -v tests.test_reserve.ReserveTests.test_concurrent_reserves_do_not_oversell 2>&1 | tail -n 3
    EXPECT: /^OK$/m
    FROM: R1 "has a test that reproduces it and fails"
- [ ] G2: a release racing a reserve is not lost
    CHECK: python3 -c "exec('import threading as th\nfrom inventory import Inventory, MemoryStore\ngo=th.Event(); done=th.Event()\nclass S(MemoryStore):\n    n=0\n    def get(self, k):\n        v=super().get(k); S.n+=1\n        if S.n==1: go.set(); done.wait(0.5)\n        return v\ninv=Inventory(S({\"A\":5}))\nb=th.Thread(target=lambda: (go.wait(2), inv.release(\"A\",2), done.set()))\nb.start(); inv.reserve(\"A\",3); go.set(); b.join()\nprint(\"stock\", inv.available(\"A\"))')"
    EXPECT: /^stock 4$/m
    FROM: R2 "safe to call from several threads at once"
- [ ] G3: the public methods are unchanged
    CHECK: python3 -c "import inspect; from inventory.reserve import Inventory; print(*(f'{m}{inspect.signature(getattr(Inventory,m))}' for m in ('reserve','release','available')))"
    EXPECT: reserve(self, sku: str, qty: int) -> int release(self, sku: str, qty: int) -> int available(self, sku: str) -> int
    FROM: R2 "without changing its public methods"
    RED: none
