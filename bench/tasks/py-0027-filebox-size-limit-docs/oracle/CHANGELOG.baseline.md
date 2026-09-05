<!-- canary: 57c8933b1286efcf -->
# Changelog

## Unreleased

## 0.7.2

- `load` raises `E_MISSING` instead of `FileNotFoundError`.

## 0.7.1

- `save` rejects names with path separators (`E_NAME`).
- `save` refuses to overwrite an existing file (`E_EXISTS`).
