import threading
import time
import unittest

from inventory import Inventory, MemoryStore, OutOfStock


class SlowStore(MemoryStore):
    """A store whose reads take a while, like the network-backed one."""

    def get(self, sku):
        value = super().get(sku)
        time.sleep(0.05)  # the network round trip happens after the read is answered
        return value


class ReserveTests(unittest.TestCase):
    def test_reserve_takes_units(self):
        inv = Inventory(MemoryStore({"SKU-1": 5}))
        self.assertEqual(inv.reserve("SKU-1", 3), 2)
        self.assertEqual(inv.available("SKU-1"), 2)

    def test_reserve_refuses_more_than_stock(self):
        inv = Inventory(MemoryStore({"SKU-1": 2}))
        with self.assertRaises(OutOfStock):
            inv.reserve("SKU-1", 3)

    def test_concurrent_reserves_do_not_oversell(self):
        inv = Inventory(SlowStore({"SKU-4471": 5}))
        errors = []

        def order():
            try:
                inv.reserve("SKU-4471", 3)
            except OutOfStock as e:
                errors.append(e)

        threads = [threading.Thread(target=order) for _ in range(2)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        self.assertEqual(len(errors), 1, "exactly one of the two orders must be refused")
        self.assertEqual(inv.available("SKU-4471"), 2)
