"""Error codes shown to intake users. Every code has a row in docs/errors.md."""

ERROR_CODES = {
    "E_NAME": "the file name is empty or contains a path separator",
    "E_EXISTS": "a file with that name is already stored",
    "E_MISSING": "no file with that name is stored",
}


class FileboxError(Exception):
    def __init__(self, code: str, message: str | None = None):
        if code not in ERROR_CODES:
            raise KeyError(f"unregistered error code {code}")
        self.code = code
        super().__init__(message or ERROR_CODES[code])
