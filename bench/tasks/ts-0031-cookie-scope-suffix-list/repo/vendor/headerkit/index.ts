// headerkit 1.1.0, vendored under the licence review (see docs/adr/0007).
// Serialises a cookie attribute set into a Set-Cookie header value.

export type CookieOptions = {
  domain?: string;
  path?: string;
  httpOnly?: boolean;
  sameSite?: "Lax" | "Strict" | "None";
};

export function serialize(name: string, value: string, opts: CookieOptions = {}): string {
  const parts = [name + "=" + encodeURIComponent(value)];
  if (opts.domain) parts.push("Domain=" + opts.domain);
  if (opts.path) parts.push("Path=" + opts.path);
  if (opts.httpOnly) parts.push("HttpOnly");
  if (opts.sameSite) parts.push("SameSite=" + opts.sameSite);
  return parts.join("; ");
}
