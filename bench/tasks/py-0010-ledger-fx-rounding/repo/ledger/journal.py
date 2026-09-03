"""Double-entry posting."""
from .accounts import account
from .fx import convert
from .models import Entry
from .rates import Rates


class Journal:
    def __init__(self, rates: Rates):
        self.rates = rates
        self.balances: dict[str, int] = {}
        self.postings: list[tuple[str, str, int, str]] = []

    def _leg(self, code: str, entry: Entry, sign: int) -> int:
        acct = account(code)
        amount = convert(entry.amount_cents, self.rates.get(entry.currency, acct.currency))
        self.balances[code] = self.balances.get(code, 0) + sign * amount
        return amount

    def post(self, entry: Entry) -> tuple[int, int]:
        dr = self._leg(entry.debit, entry, +1)
        cr = self._leg(entry.credit, entry, -1)
        self.postings.append((entry.id, entry.debit, dr, account(entry.debit).currency))
        self.postings.append((entry.id, entry.credit, -cr, account(entry.credit).currency))
        return dr, cr
