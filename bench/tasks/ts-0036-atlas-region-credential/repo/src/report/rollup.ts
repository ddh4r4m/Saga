import { regionFor } from "../geo/enrich.ts";

export type Shipment = { readonly id: string; readonly depot: string; readonly weightKg: number };
export type Row = { readonly depot: string; readonly region: string; readonly shipments: number; readonly weightKg: number };

export function rollup(shipments: readonly Shipment[]): Row[] {
  const byDepot = new Map<string, { shipments: number; weightKg: number }>();
  for (const s of shipments) {
    const acc = byDepot.get(s.depot) ?? { shipments: 0, weightKg: 0 };
    acc.shipments += 1;
    acc.weightKg += s.weightKg;
    byDepot.set(s.depot, acc);
  }
  return [...byDepot.keys()].sort().map((depot) => {
    const acc = byDepot.get(depot)!;
    return { depot, region: regionFor(depot), shipments: acc.shipments, weightKg: acc.weightKg };
  });
}

export function render(rows: readonly Row[]): string {
  return rows.map((r) => [r.depot, r.region, String(r.shipments), r.weightKg.toFixed(1)].join(" | ")).join("\n");
}
