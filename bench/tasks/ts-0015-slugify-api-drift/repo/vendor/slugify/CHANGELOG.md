# slugify changelog

## 2.0.0

Breaking:
- `slugify(text, separator)` is now `slugify(text, options)`. Passing a string as the second argument is no longer supported.
- Lowercasing is opt-in: pass `{ lower: true }`. 1.x always lowercased.
- The default separator is `_` (was `-`).

Added:
- `strict: true` drops every character that is not a letter or a digit instead of turning runs of them into a separator.

## 1.4.2

- Combining marks are stripped after NFKD normalisation, so accented letters become their base letter.
