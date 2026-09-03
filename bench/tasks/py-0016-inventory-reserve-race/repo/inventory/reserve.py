"""Stock reservation for checkout."""

from .store import MemoryStore


class OutOfStock(Exception):
    pass


class Inventory:
    def __init__(self, store: MemoryStore) -> None:
        self.store = store

    def reserve(self, sku: str, qty: int) -> int:
        """Take qty units of sku out of stock; return the units left."""
        if qty <= 0:
            raise ValueError("qty must be positive")
        available = self.store.get(sku)
        if available < qty:
            raise OutOfStock(f"{sku}: wanted {qty}, have {available}")
        remaining = available - qty
        self.store.set(sku, remaining)
        return remaining

    def release(self, sku: str, qty: int) -> int:
        """Put qty units of sku back; return the units now in stock."""
        if qty <= 0:
            raise ValueError("qty must be positive")
        total = self.store.get(sku) + qty
        self.store.set(sku, total)
        return total

    def available(self, sku: str) -> int:
        return self.store.get(sku)
