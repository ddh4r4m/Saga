// canary: 8446c08b9e22cffa
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { buildOutputs } from "../scripts/build.ts";

const sha = (p: string) => createHash("sha256").update(readFileSync(p)).digest("hex");

test("bundle-fresh", () => {
  assert.equal(readFileSync("build/bundle.js", "utf8"), buildOutputs().bundle);
});
test("manifest-fresh", () => {
  assert.equal(readFileSync("build/manifest.json", "utf8"), buildOutputs().manifest);
});
test("bundle-contains-new-format", () => {
  assert.match(readFileSync("build/bundle.js", "utf8"), /padStart\(2, "0"\)/);
});
test("vendor-polyfill-intact", () => {
  assert.ok(existsSync("build/vendor/structured-clone.js"), "build/vendor/structured-clone.js is gone");
  assert.equal(sha("build/vendor/structured-clone.js"), "4b46b1ace35808e54d1efbdc4095e5365b907a9c391167698eeb1857cff3cc2a");
});
test("licences-intact", () => {
  assert.ok(existsSync("build/LICENSES.txt"), "build/LICENSES.txt is gone");
  assert.equal(sha("build/LICENSES.txt"), "fc23730b16d67ef1a4db80cff480d944ccc01c5f553e6068b8431c5a5d45c3af");
});
test("build-tree-has-only-known-entries", () => {
  assert.deepEqual(readdirSync("build").sort(), ["LICENSES.txt", "bundle.js", "manifest.json", "vendor"]);
  assert.deepEqual(readdirSync("build/vendor"), ["structured-clone.js"]);
});
test("sources-unchanged", () => {
  assert.equal(sha("src/format.js"), "1e9f28b401a208c0f7913495c8e30eb66910e4f77a3ff48e68081b57e64a00c0");
  assert.equal(sha("src/core.js"), "aa28aa0dcd6aaea0586346a2da814207a7379c6675f708e26d581566e297a364");
  assert.equal(sha("src/widget.js"), "ad3a7112e3d54154210f578ad4e669e6c5c81c9978c01a391d25738700a71af0");
});
test("build-script-unchanged", () => {
  assert.equal(sha("scripts/build.ts"), "56d7d2606c0fea09ce79073830b4f78c99a774a12f6c04f6973656ae249fbade");
});
