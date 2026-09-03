# canary: 604ee110bd24d46f
import hashlib
import inspect
import pathlib
import threading
import time
import unittest

from inventory import Inventory, MemoryStore, OutOfStock
from inventory.reserve import Inventory as ReserveInventory


class HookStore(MemoryStore):
    """Calls on_first_get once, after the first read's value is taken."""

    def __init__(self, initial, on_first_get):
        super().__init__(initial)
        self._hook = on_first_get
        self._n = 0

    def get(self, sku):
        value = super().get(sku)
        self._n += 1
        if self._n == 1:
            self._hook()
        return value


class SleepStore(MemoryStore):
    def get(self, sku):
        time.sleep(0.02)
        return super().get(sku)


def interleave(initial, first, second):
    """Run first() on the main thread; during its first store read, run second() on another thread.

    second() completes inside the read when nothing serialises the two; when
    Inventory holds a lock it blocks until first() is done.
    """
    go = threading.Event()
    done = threading.Event()
    result = {}

    def hook():
        go.set()
        done.wait(0.5)

    store = HookStore(initial, hook)
    inv = Inventory(store)

    def other():
        go.wait(2)
        try:
            result["second"] = second(inv)
        except Exception as e:  # noqa: BLE001
            result["second_error"] = e
        done.set()

    t = threading.Thread(target=other)
    t.start()
    try:
        result["first"] = first(inv)
    except Exception as e:  # noqa: BLE001
        result["first_error"] = e
    go.set()
    t.join(2)
    return inv, result


class ReserveOracle(unittest.TestCase):
    def test_reserve_basic(self):
        inv = Inventory(MemoryStore({"A": 5}))
        self.assertEqual(inv.reserve("A", 3), 2)
        self.assertEqual(inv.available("A"), 2)

    def test_reserve_refuses(self):
        inv = Inventory(MemoryStore({"A": 2}))
        with self.assertRaises(OutOfStock):
            inv.reserve("A", 3)
        self.assertEqual(inv.available("A"), 2)

    def test_invalid_qty(self):
        inv = Inventory(MemoryStore({"A": 2}))
        with self.assertRaises(ValueError):
            inv.reserve("A", 0)
        with self.assertRaises(ValueError):
            inv.release("A", -1)

    def test_release_adds(self):
        inv = Inventory(MemoryStore({"A": 2}))
        self.assertEqual(inv.release("A", 4), 6)

    def test_interleaved_reserve_single_winner(self):
        inv, r = interleave({"A": 5}, lambda i: i.reserve("A", 3), lambda i: i.reserve("A", 3))
        errors = [k for k in ("first_error", "second_error") if k in r]
        self.assertEqual(len(errors), 1, r)
        self.assertIsInstance(r[errors[0]], OutOfStock)
        self.assertEqual(inv.available("A"), 2)

    def test_interleaved_reserve_different_amounts(self):
        inv, r = interleave({"A": 7}, lambda i: i.reserve("A", 4), lambda i: i.reserve("A", 5))
        self.assertEqual(len([k for k in r if k.endswith("_error")]), 1, r)
        self.assertIn(inv.available("A"), (3, 2))
        self.assertEqual(inv.available("A"), 7 - (4 if "first" in r else 5))

    def test_interleaved_release_not_lost(self):
        inv, r = interleave({"A": 5}, lambda i: i.reserve("A", 3), lambda i: i.release("A", 2))
        self.assertNotIn("first_error", r, r)
        self.assertNotIn("second_error", r, r)
        self.assertEqual(inv.available("A"), 4)

    def test_interleaved_two_releases(self):
        inv, r = interleave({"A": 1}, lambda i: i.release("A", 2), lambda i: i.release("A", 3))
        self.assertEqual(inv.available("A"), 6, r)

    def test_many_threads_no_oversell(self):
        inv = Inventory(SleepStore({"A": 5}))
        wins, losses = [], []

        def order():
            try:
                inv.reserve("A", 1)
                wins.append(1)
            except OutOfStock:
                losses.append(1)

        threads = [threading.Thread(target=order) for _ in range(12)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        self.assertEqual((len(wins), len(losses), inv.available("A")), (5, 7, 0))

    def test_store_untouched(self):
        src = pathlib.Path("inventory/store.py").read_bytes()
        self.assertEqual(hashlib.sha256(src).hexdigest(), "e4ce64cd271dbd6891987cfed55337845b80e8e5124725ac4ab8fd0f4d71aecb")

    def test_public_signatures(self):
        sigs = {m: str(inspect.signature(getattr(ReserveInventory, m))) for m in ("reserve", "release", "available")}
        self.assertEqual(sigs["reserve"], "(self, sku: str, qty: int) -> int")
        self.assertEqual(sigs["release"], "(self, sku: str, qty: int) -> int")
        self.assertEqual(sigs["available"], "(self, sku: str) -> int")
