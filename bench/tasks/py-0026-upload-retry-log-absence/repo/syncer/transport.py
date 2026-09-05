"""Transport used by upload(); the real one wraps urllib, tests pass fakes."""


class TransportError(Exception):
    """Raised by a transport when a request fails."""


class HttpTransport:
    def send(self, url: str, headers: dict[str, str], body: bytes) -> dict:
        import json
        import urllib.request

        req = urllib.request.Request(url, data=body, headers=headers, method="PUT")
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except OSError as err:
            raise TransportError(str(err)) from err
