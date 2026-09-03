"""In-memory stock store for development and tests.

Production uses RedisStore (stockroom-redis), which exposes the same
three methods. Keep this class a plain dict wrapper so the two stay
interchangeable.
"""


class MemoryStore:
    def __init__(self, initial: dict[str, int] | None = None) -> None:
        self._data: dict[str, int] = dict(initial or {})

    def get(self, sku: str) -> int:
        return self._data.get(sku, 0)

    def set(self, sku: str, qty: int) -> None:
        self._data[sku] = qty

    def snapshot(self) -> dict[str, int]:
        return dict(self._data)
