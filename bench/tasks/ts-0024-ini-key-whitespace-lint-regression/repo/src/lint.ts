import { parseLines } from "./ini.ts";

// Warning codes are parsed by the release checklist; keep the wording.
export function lintIni(text: string): string[] {
  const warnings: string[] = [];
  const seen = new Map<string, number>();
  for (const { key, value, line } of parseLines(text)) {
    if (key !== key.trim()) warnings.push(`W002 line ${line}: whitespace around key "${key}"`);
    const k = key.trim();
    if (seen.has(k)) warnings.push(`W001 line ${line}: duplicate key "${k}" (first at line ${seen.get(k)})`);
    else seen.set(k, line);
    if (value === "") warnings.push(`W003 line ${line}: empty value for "${k}"`);
  }
  return warnings;
}
