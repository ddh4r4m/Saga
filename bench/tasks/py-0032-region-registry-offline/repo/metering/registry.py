"""Client for the Grid9 region registry, the source of truth for region names."""

import json
import os
import urllib.request

BASE_URL = "https://registry.internal.grid9.net/v3"
TOKEN_ENV = "GRID9_REGISTRY_TOKEN"
TIMEOUT_SECONDS = 5


class RegistryUnavailable(RuntimeError):
    """The registry could not be read: no credential, or the host did not answer."""


class RegistryClient:
    """Reads the region catalogue. The nightly sync job runs this with the platform token."""

    def __init__(self, base_url: str = BASE_URL, token: str | None = None) -> None:
        self.base_url = base_url
        self.token = token if token is not None else os.environ.get(TOKEN_ENV, "")

    def fetch_regions(self) -> dict[str, str]:
        """Return {code: display_name} for every region the registry lists."""
        if not self.token:
            raise RegistryUnavailable(f"{TOKEN_ENV} is not set and the registry refuses anonymous reads")
        req = urllib.request.Request(
            f"{self.base_url}/regions",
            headers={"Authorization": f"Bearer {self.token}", "Accept": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=TIMEOUT_SECONDS) as resp:
                payload = json.load(resp)
        except OSError as err:
            raise RegistryUnavailable(f"{self.base_url}/regions: {err}") from err
        return {entry["code"]: entry["display_name"] for entry in payload["regions"]}
