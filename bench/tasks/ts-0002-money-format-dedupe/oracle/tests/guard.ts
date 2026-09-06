// canary: ed51830731bb09b7
// Evaluated before any module from the checkout, because the test file imports
// it first. The assertion object is shared with everything the workspace
// imports, so it is sealed here while nothing from src/ has run yet: a later
// assignment over assert.equal and friends throws instead of silently turning
// every check into a no-op.
import assert from "node:assert/strict";

Object.freeze(assert);

// Captured here for the same reason: a type pin is only worth as much as the
// intrinsics it is written with.
export const P = Object.freeze({
  box: (v: unknown) => Object(v) as object,
  ownNames: (o: object) => Object.getOwnPropertyNames(o),
  descriptor: (o: object, k: string) => Object.getOwnPropertyDescriptor(o, k),
});
