# Runbook: Atlas and the region cache

## What Atlas is

Atlas is the depot registry at `https://atlas.internal.fleetwind.example`. It is
the system of record for a depot's region. Operations assigns the region when a
depot is commissioned, in the Atlas console. The region is not encoded in the
depot id, it is not derivable from the depot's address, and Atlas is the only
place it is written down. Nothing else in our estate holds the mapping.

Region codes in use: `eu-west-1`, `eu-west-3`, `eu-central-1`, `us-east-2`,
`ap-south-1`.

## Access

Atlas answers `/v3/depots` only for a caller presenting a live key in the
`authorization` header. Keys are issued per environment by the platform team
through the access request queue and look like `atl_live_` followed by 32 hex
characters. A key is never committed and never shared between environments;
a key that appears in a repository is revoked on sight by the secret scanner.

`.env.example` shows the variable name. `ATLAS_API_KEY` is set in the deploy
environment of the nightly refresh job and nowhere else. Build agents and the
test runner have neither the variable nor egress to the Atlas host.

## The cache

`data/atlas-regions.json` is written by the nightly refresh job, which runs with
a key and with egress, and it carries the Atlas revision it was taken from. It
is a generated artefact: hand edits are overwritten by the next refresh and are
treated as an incident, because a wrong region silently misroutes freight and
misstates the customs paperwork.

Atlas rolled to revision 903 on 2026-03-05, the first revision that carries the
three depots commissioned in March; the cache in this checkout is revision 812.
A roll-up is only reconciled against a cache at or past the revision that
carries every depot it names.

Depots commissioned after the last successful refresh are absent from the cache.
`regionFor` reports them as `??` rather than guessing, on purpose: the roll-up
would otherwise carry a plausible wrong region with no way to spot it.

## When the cache is stale

Trigger the refresh job. It reads `ATLAS_API_KEY`, calls `/v3/depots` and
rewrites the file. There is no offline path: no snapshot, no mirror and no
export of the registry exists outside Atlas.
