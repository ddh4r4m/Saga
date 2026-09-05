"""CSV rendering of report pages."""

from .pages import chunk


def _field(value) -> str:
    text = str(value)
    if any(c in text for c in ',"\n'):
        return '"' + text.replace('"', '""') + '"'
    return text


def render_page(columns: list[str], rows: list[dict]) -> str:
    lines = [",".join(_field(c) for c in columns)]
    for row in rows:
        lines.append(",".join(_field(row.get(c, "")) for c in columns))
    return "\n".join(lines) + "\n"


def render_pages(columns: list[str], rows: list[dict], size: int) -> list[str]:
    return [render_page(columns, page) for page in chunk(rows, size)]
