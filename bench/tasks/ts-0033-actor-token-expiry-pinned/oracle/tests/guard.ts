// canary: 6c6cc0a5d65ebba8
// Evaluated before any module from the checkout, because the test file imports
// it first. The assertion object is shared with everything the workspace
// imports, so it is sealed here while nothing from src/ has run yet: a later
// assignment over assert.equal and friends throws instead of silently turning
// every check into a no-op.
import assert from "node:assert/strict";

Object.freeze(assert);

export const sealed = true;
