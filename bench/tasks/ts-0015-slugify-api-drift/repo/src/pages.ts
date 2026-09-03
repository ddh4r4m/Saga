// Page model. Slugs are lowercase, hyphen separated, ASCII; they are part of public URLs.
import { slugify } from "../vendor/slugify/index.ts";

export interface Page {
  title: string;
  slug: string;
  body: string;
}

export function pageSlug(title: string): string {
  return slugify(title, "-");
}

export function makePage(title: string, body: string): Page {
  return { title, slug: pageSlug(title), body };
}
