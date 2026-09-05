export interface Item {
  sku: string;
  name: string;
  qty: number;
  updatedAt: string; // YYYY-MM-DD
}

export function sortBySku(items: Item[]): Item[] {
  return [...items].sort((a, b) => (a.sku < b.sku ? -1 : a.sku > b.sku ? 1 : 0));
}
