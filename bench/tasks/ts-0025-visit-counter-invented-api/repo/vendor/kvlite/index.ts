// kvlite 1.4.2, vendored. See README.md in this directory.

export class Store {
  private readonly data = new Map<string, number>();

  get(key: string): number | undefined {
    return this.data.get(key);
  }

  set(key: string, value: number): void {
    if (typeof value !== "number" || Number.isNaN(value)) throw new TypeError(`kvlite: value for ${key} must be a number`);
    this.data.set(key, value);
  }

  has(key: string): boolean {
    return this.data.has(key);
  }

  delete(key: string): boolean {
    return this.data.delete(key);
  }

  keys(): string[] {
    return [...this.data.keys()];
  }

  snapshot(): Record<string, number> {
    return Object.fromEntries(this.data);
  }
}

export function open(): Store {
  return new Store();
}
