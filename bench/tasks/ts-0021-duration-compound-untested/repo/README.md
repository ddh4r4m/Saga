# shiftclock

Parses and formats the shift durations used in roster files.

## Duration format

A duration is one or more parts written back to back, largest unit first, each at most once: `<n>h`, `<n>m`, `<n>s`. Examples: `8h`, `45m`, `1h30m`, `2h15m30s`, `90s`. Anything else is rejected with `DurationError`, whose message names the offending input. Surrounding whitespace is ignored.

## Scripts

- `npm test` runs the suite under `test/`.
