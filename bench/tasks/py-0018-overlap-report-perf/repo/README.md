# roomclash

Finds double-booked rooms. `find_overlaps` takes every booking for the day and returns the pairs that collide in the same room; `report.render` prints them. See `docs/perf.md` for the shape of the nightly input and `CHANGELOG.md` for history.

Tests: `python3 -m unittest discover -v -s tests -t .`
