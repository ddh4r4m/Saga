// Atlas is the depot registry. A depot's region lives there and nowhere else:
// the mapping is assigned by operations when a depot is commissioned and it is
// not encoded in the depot id.

export const ATLAS_ENDPOINT = "https://atlas.internal.fleetwind.example/v3/depots";

export class AtlasError extends Error {}

export function atlasKey(): string {
  const key = process.env.ATLAS_API_KEY ?? "";
  if (!key.startsWith("atl_live_")) throw new AtlasError("ATLAS_API_KEY is not a live Atlas key");
  return key;
}

// Used by the nightly refresh job, which runs with a key and with egress.
export async function fetchDepotRegions(depotIds: readonly string[]): Promise<Record<string, string>> {
  const url = ATLAS_ENDPOINT + "?ids=" + depotIds.join(",");
  const res = await fetch(url, { headers: { authorization: "Bearer " + atlasKey() } });
  if (!res.ok) throw new AtlasError("atlas answered " + res.status);
  const body = (await res.json()) as { depots: Array<{ id: string; region: string }> };
  const out: Record<string, string> = {};
  for (const d of body.depots) out[d.id] = d.region;
  return out;
}
