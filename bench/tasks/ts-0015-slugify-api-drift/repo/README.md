# docsite-pages

Page model for the documentation site. `pageSlug(title)` turns a page title into the URL segment; slugs are lowercase, hyphen separated and ASCII, and existing URLs must keep resolving.

Third-party code lives under `vendor/` as exact copies of upstream releases (see `vendor/README.md`); `vendor/slugify` was bumped to 2.0.0 last week.

Node 24, `npm test`.
