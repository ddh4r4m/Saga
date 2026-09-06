# metering

The monthly consumption report behind the billing run. `metering.monthly_consumption(readings, month)` takes the field readings for a period and a month as `YYYY-MM`, and returns a mapping of meter id to the kilowatt hours consumed in that month.

Rules (`docs/readings.md` has the long version):
- each meter's readings are sorted by date and walked in consecutive pairs
- a pair counts when the later reading of the pair was taken in the reported month, so the first pair of a month usually straddles the month boundary
- a meter with no counted pair does not appear in the report

Run the tests with `python3 -m unittest discover -s tests -t .`.
