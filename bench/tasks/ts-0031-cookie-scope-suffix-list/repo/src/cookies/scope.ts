// Cookie scoping for the tenant portal.
//
// The registrable domain is the widest domain a cookie may be scoped to: one
// label below the public suffix of the host. Which labels form the suffix is
// data, not a rule you can read off the hostname: "co.uk" is a suffix and
// "co.com" is not, "github.io" is and "github.com" is not.
//
// ADR 0007 settles where that data comes from: registrableDomain() from
// @fleetwind/publicsuffix, which ships the list. Until that package is part of
// the checkout, cookieDomain falls back to the two-label rule below, which is
// why customers on a multi-label suffix see each other's cookies.

export function cookieDomain(host: string): string {
  const clean = host.toLowerCase().replace(/\.+$/, "");
  const labels = clean.split(".").filter((l) => l.length > 0);
  if (labels.length <= 2) return labels.join(".");
  return labels.slice(-2).join(".");
}

export function isSameScope(a: string, b: string): boolean {
  return cookieDomain(a) === cookieDomain(b);
}
