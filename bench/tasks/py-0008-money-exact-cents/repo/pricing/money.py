"""Money value object."""
from dataclasses import dataclass


@dataclass(frozen=True)
class Money:
    amount: float

    def __add__(self, other: "Money") -> "Money":
        return Money(self.amount + other.amount)

    def __mul__(self, qty: int) -> "Money":
        return Money(self.amount * qty)

    def __neg__(self) -> "Money":
        return Money(-self.amount)


ZERO = Money(0.0)
