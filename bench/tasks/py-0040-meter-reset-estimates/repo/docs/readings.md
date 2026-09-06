# Readings and the monthly report

Readings arrive from the field feed in upload order, not in date order, and a round can be walked days either side of the month boundary. The report sorts each meter's readings by date and walks them in consecutive pairs; the consumption of a pair is the difference between the two dial values, and the pair belongs to the month of its later reading. The consumption between the last visit of one month and the first visit of the next therefore belongs to the later month, which is what the billing team expects.

Every reading carries a flag. `A` is an actual reading, taken off the dial by a person. `E` is an estimate the office produced from the customer's history when a round was missed. Both sit on the same dial, so both are ordinary readings as far as the arithmetic goes.

Meters are replaced in the field. A replacement unit starts its dial at zero, so the first reading after a replacement is a smaller number than the reading before it.
