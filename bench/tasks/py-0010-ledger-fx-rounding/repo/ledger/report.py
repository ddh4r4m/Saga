"""Text output helpers."""


def fmt_cents(cents: int, currency: str) -> str:
    sign = "-" if cents < 0 else ""
    return f"{sign}{abs(cents) // 100}.{abs(cents) % 100:02d} {currency}"


def posting_line(entry_id: str, date: str, debit: str, credit: str, cents: int, currency: str) -> str:
    return f"post {entry_id} {date} dr {debit} cr {credit} {fmt_cents(cents, currency)}"


def balance_line(code: str, name: str, cents: int, currency: str) -> str:
    return f"balance {code} {name:<16} {fmt_cents(cents, currency):>16}"
