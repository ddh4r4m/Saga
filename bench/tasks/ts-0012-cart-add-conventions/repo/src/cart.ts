export interface Line {
  sku: string;
  qty: number;
}

export interface Cart {
  lines: Line[];
}

export function emptyCart(): Cart {
  return { lines: [] };
}
