# edgelog

Reads the CDN edge log exports under `fixtures/exports/` and rolls them up. `edgelog.reader.read_lines` decodes one export; `edgelog.summary.summarise` counts requests per status class across a set of them.

An export line is `<timestamp> <method> <path> <status> <bytes>`, separated by single spaces.

Tests: `python3 -m unittest discover -v -s tests -t .`
