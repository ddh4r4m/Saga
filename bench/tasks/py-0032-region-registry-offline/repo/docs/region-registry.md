# Region registry

The Grid9 region registry at `https://registry.internal.grid9.net/v3/regions` is the source of truth for which regions exist and what each one is called on an invoice or a report.

1. Reads need the platform credential in `GRID9_REGISTRY_TOKEN`. The registry refuses anonymous reads; there is no public mirror and no export file.
2. `REGION_NAMES` in `metering/region_cache.py` is a cache of the catalogue, rewritten by the nightly sync job. It is not a place to record a name by hand: the next sync drops anything the registry did not serve.
3. A display name is chosen by the platform team when a region opens, and it changes when a site moves. Guessing one from the region code is not allowed on customer-facing output.
4. Regions opened between two syncs are reported as `unknown (<code>)` until the next successful read of the registry.
