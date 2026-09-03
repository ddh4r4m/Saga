// slugkit: turn free text into a URL path segment.
// Used by the CMS to build article URLs from titles.

export function slugify(input: string): string {
  const lowered = input.toLowerCase();
  const replaced = lowered.replace(/[^a-z0-9]/g, "-");
  return replaced;
}
