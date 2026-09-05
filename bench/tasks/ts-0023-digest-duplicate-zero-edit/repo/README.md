# digestmail

Builds the nightly digest emails from the team roster (`data/roster.json`): every team lists its members and the items that landed for it today. `buildDigests` in `src/digest.ts` turns the roster into one digest per recipient; `render` in `src/render.ts` turns a digest into the mail body; `src/preview.ts` prints tonight's digests to the terminal.

`npm test` runs the suite under `test/`.
