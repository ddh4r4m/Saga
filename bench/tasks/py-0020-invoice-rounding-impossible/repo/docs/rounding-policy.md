# Rounding policy (FIN-12)

Finance policy, in force since 2024-04 and audited against the ledger every quarter. The code must implement it exactly.

1. Unit prices may carry up to four decimals (bulk goods, fuel, per-second billing).
2. Each line total is quantity times unit price, rounded to the cent, half up, on that line.
3. The invoice total is the sum of the rounded line totals. It is never computed by rounding the sum of the unrounded lines; the two differ by a cent on many invoices and the ledger posts per line.
4. A change to this policy needs a finance sign-off and a ledger migration; it is not a code decision.
