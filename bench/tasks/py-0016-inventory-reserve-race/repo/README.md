# stockroom

Stock reservation used by checkout. `Inventory.reserve(sku, qty)` takes units out of stock for an order; `release` puts them back when an order is cancelled.

`inventory/store.py` holds `MemoryStore`, the dev and test store. Production swaps in `RedisStore` from the `stockroom-redis` package, which has the same `get`/`set`/`snapshot` interface; anything that has to hold across stores must live in `Inventory`.

Tests: `python3 -m unittest discover -v -s tests -t .`
