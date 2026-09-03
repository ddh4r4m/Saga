// User-facing failures. See CONTRIBUTING.md, "Errors".

export const ERROR_CODES = {
  E_UNKNOWN_SKU: "unknown sku",
  E_EMPTY_CART: "cart is empty",
  E_USAGE: "usage: cartctl add <sku> <qty>",
} as const;

export type ErrorCode = keyof typeof ERROR_CODES;

export class CliError extends Error {
  readonly code: string;
  constructor(code: string, message: string) {
    super(message);
    this.name = "CliError";
    this.code = code;
  }
}

export function fail(code: string, message?: string): never {
  if (!(code in ERROR_CODES)) throw new Error(`unregistered error code ${code}`);
  throw new CliError(code, message ?? ERROR_CODES[code as ErrorCode]);
}
