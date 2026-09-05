export interface IniLine {
  key: string;
  value: string;
}

// Splits "key = value" at the first "=". The value is trimmed; the key is
// returned as written so callers can decide what to do with it.
export function parseLine(line: string): IniLine | null {
  const text = line.replace(/\r$/, "");
  if (text.trim() === "" || /^\s*[#;]/.test(text)) return null;
  const eq = text.indexOf("=");
  if (eq < 0) return null;
  return { key: text.slice(0, eq), value: text.slice(eq + 1).trim() };
}

export function parseLines(text: string): Array<IniLine & { line: number }> {
  const out: Array<IniLine & { line: number }> = [];
  text.split("\n").forEach((raw, i) => {
    const parsed = parseLine(raw);
    if (parsed) out.push({ ...parsed, line: i + 1 });
  });
  return out;
}
