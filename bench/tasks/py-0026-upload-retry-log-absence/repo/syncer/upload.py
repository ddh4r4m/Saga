"""Upload a report file to the sync service."""

import logging
import os

from .transport import TransportError

log = logging.getLogger("syncer.upload")

SERVICE = "https://sync.example.net/v1/files"


class UploadError(Exception):
    """Raised when the upload cannot be completed."""


def upload(path: str, token: str, transport) -> dict:
    """Send the file at path with the bearer token and return the service's reply."""
    with open(path, "rb") as fh:
        body = fh.read()
    name = os.path.basename(path)
    url = f"{SERVICE}?name={name}"
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/octet-stream"}
    try:
        return transport.send(url, headers, body)
    except TransportError as err:
        raise UploadError(f"upload of {name} failed: {err}") from err
