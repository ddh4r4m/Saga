"""Chart of accounts."""
from .models import Account

CHART = {
    "1000": Account("1000", "Cash", "USD"),
    "1200": Account("1200", "Receivables", "USD"),
    "2000": Account("2000", "Payables", "USD"),
    "3000": Account("3000", "Revenue", "USD"),
    "4000": Account("4000", "Expenses", "USD"),
    "4471": Account("4471", "EU Receivables", "EUR"),
    "6000": Account("6000", "Bank fees", "USD"),
}


def account(code: str) -> Account:
    try:
        return CHART[code]
    except KeyError:
        raise KeyError(f"unknown account {code}") from None
