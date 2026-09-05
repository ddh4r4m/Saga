"""Split rows into fixed-size pages."""


def chunk(rows: list, size: int) -> list[list]:
    """Pages of size rows each, in order; the last page may be shorter."""
    if size <= 0:
        raise ValueError("page size must be positive")
    pages = []
    for start in range(0, len(rows) - size, size):
        pages.append(rows[start : start + size])
    return pages
