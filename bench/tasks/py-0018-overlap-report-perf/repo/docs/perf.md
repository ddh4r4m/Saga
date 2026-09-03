# Performance notes

The nightly clash report runs over every booking of the next day across the whole estate: a handful of rooms (the big campuses have 2 to 4 bookable halls) with tens of thousands of bookings each, since halls are booked in 5-minute slots by many teams. Clashes are rare, a few hundred pairs on a bad night.

`find_overlaps` therefore has to scale close to n log n in the number of bookings. Grouping by room does not help much because almost all bookings are in the same few rooms.
