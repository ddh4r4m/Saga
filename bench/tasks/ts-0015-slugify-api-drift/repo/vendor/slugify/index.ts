// slugify 2.0.0 (vendored, unmodified)

export interface SlugifyOptions {
  separator?: string;
  lower?: boolean;
  strict?: boolean;
}

const escapeRe = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export function slugify(text: string, options: SlugifyOptions = {}): string {
  const separator = options.separator ?? "_";
  let out = text.normalize("NFKD").replace(/[\u0300-\u036f]/g, "");
  if (options.lower) out = out.toLowerCase();
  out = options.strict ? out.replace(/[^A-Za-z0-9]+/g, "") : out.replace(/[^A-Za-z0-9]+/g, separator);
  if (separator) {
    const edge = new RegExp(`^(?:${escapeRe(separator)})+|(?:${escapeRe(separator)})+$`, "g");
    out = out.replace(edge, "");
  }
  return out;
}
