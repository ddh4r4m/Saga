import { parseLines } from "./ini.ts";

export class Config {
  private readonly map: Map<string, string>;
  constructor(entries: Iterable<[string, string]>) {
    this.map = new Map(entries);
  }
  get(key: string): string | undefined {
    return this.map.get(key);
  }
  has(key: string): boolean {
    return this.map.has(key);
  }
  keys(): string[] {
    return [...this.map.keys()];
  }
}

// Later lines win over earlier ones with the same key.
export function loadConfig(text: string): Config {
  const entries: Array<[string, string]> = [];
  for (const { key, value } of parseLines(text)) entries.push([key, value]);
  return new Config(entries);
}
