# reports

Nightly report exporter. `reports.export` renders the rows the nightly job collects into the CSV pages the portal serves; `reports.pages.chunk` splits the rows into pages of a fixed size.

`make test` runs the suite (unittest discovery under `tests/`); CI runs the same target.
