"""File storage under a root directory."""

import os

from .errors import FileboxError

MAX_BYTES = 5 * 1024 * 1024

_root = os.environ.get("FILEBOX_ROOT", os.path.join(os.getcwd(), ".filebox"))


def set_root(path: str) -> None:
    global _root
    _root = path


def _path(name: str) -> str:
    if not name or "/" in name or "\\" in name or name in (".", ".."):
        raise FileboxError("E_NAME")
    return os.path.join(_root, name)


def save(name: str, data: bytes) -> int:
    """Store data under name and return the number of bytes written."""
    path = _path(name)
    if os.path.exists(path):
        raise FileboxError("E_EXISTS")
    os.makedirs(_root, exist_ok=True)
    with open(path, "wb") as fh:
        fh.write(data)
    return len(data)


def load(name: str) -> bytes:
    path = _path(name)
    try:
        with open(path, "rb") as fh:
            return fh.read()
    except FileNotFoundError:
        raise FileboxError("E_MISSING") from None
