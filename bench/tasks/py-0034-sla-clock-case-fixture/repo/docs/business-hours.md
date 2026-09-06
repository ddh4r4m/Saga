# Business hours and the SLA clock

The contract our support plans are sold under, in force since 2024-09.

1. The clock runs 09:00 to 18:00, Monday to Friday, in the support region's local time.
2. It does not run on a company holiday. The holiday list is the one in `slaclock/calendar.py`; adding or removing a date there changes what customers are owed and needs a contract amendment.
3. A ticket opened outside business hours starts its clock at 09:00 on the next day the clock runs.
4. An SLA is a number of business hours. The due instant is reached by spending those hours inside the windows above, rolling on to the next open day whenever a day runs out.
