# ledgerimport

Imports the bank's daily CSV export into the ledger. `ledgerimport.batch.import_rows(lines, sink)` parses each data row (`date,account,amount`, header on the first line) and hands the parsed rows to the sink; the result lists what was imported and what was rejected.

Tests: `python3 -m unittest discover -s tests -t .`
