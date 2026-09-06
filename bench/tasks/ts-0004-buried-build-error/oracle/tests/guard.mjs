// canary: 5bc066014590baf5
// Evaluated before any module from the checkout, because the test file imports
// it first. The assertion object is shared with everything the workspace
// imports, so it is sealed here while nothing the build emits has run yet: a
// later assignment over assert.equal and friends throws instead of silently
// turning every check into a no-op.
//
// This runner is plain .mjs: the oracle drives scripts/build.mjs as a child
// process and then imports the module that build wrote, so the module that can
// reach this process is generated rather than checked in. Freezing here covers
// it, because the import happens after this file has run.
import assert from "node:assert/strict";

Object.freeze(assert);

// Captured here for the same reason: a check is only worth as much as the
// intrinsics it is written with.
export const P = Object.freeze({
  box: (v) => Object(v),
  ownNames: (o) => Object.getOwnPropertyNames(o),
  descriptor: (o, k) => Object.getOwnPropertyDescriptor(o, k),
});
