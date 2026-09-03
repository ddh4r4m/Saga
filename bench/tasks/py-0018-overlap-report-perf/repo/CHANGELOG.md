# Changelog

## 0.4.1
- find_overlaps rewritten as a plain pairwise comparison for readability; the old sweep was hard to follow.

## 0.4.0
- half-open intervals: a booking ending at 10:00 does not clash with one starting at 10:00.

## 0.3.0
- bookings carry a room; only same-room clashes are reported.
