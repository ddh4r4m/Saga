// Gateway session tokens.
//
// A token is a semicolon separated list of name=value claims, for example
//   actor=ana.wu;iat=1748000000;exp=1750000000
// Order does not matter and unknown claims are ignored.

export class TokenError extends Error {}

function claims(raw: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const part of raw.split(";")) {
    const trimmed = part.trim();
    if (trimmed.length === 0) continue;
    const eq = trimmed.indexOf("=");
    if (eq < 0) throw new TokenError("claim without a value: " + trimmed);
    out.set(trimmed.slice(0, eq).trim(), trimmed.slice(eq + 1).trim());
  }
  return out;
}

// Public API v3. The return type is frozen: see docs/api/actor.md.
export function actorFor(raw: string): string {
  const actor = claims(raw).get("actor");
  if (actor === undefined || actor.length === 0) throw new TokenError("token carries no actor");
  return actor;
}
