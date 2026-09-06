// canary: 6f6bb515f74d8bcc
import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { gunzipSync } from "node:zlib";
import { parseBytes } from "bytesize-parse";
import { PLAN_LIMITS, fitsPlan, planBytes, remainingBytes } from "../src/quota.ts";

const OLD_TARBALL = "vendor/bytesize-parse-1.2.0.tgz";
const NEW_TARBALL = "vendor/bytesize-parse-1.3.0.tgz";
const INSTALLED = "node_modules/bytesize-parse";

function sha256(bytes: Buffer): string {
  return "sha256:" + createHash("sha256").update(bytes).digest("hex");
}

function integrityOf(path: string): string {
  return "sha512-" + createHash("sha512").update(readFileSync(path)).digest("base64");
}

// The files a npm-packed tarball carries, keyed by their path inside the
// package (npm prefixes every entry with "package/").
function tarFiles(path: string): Map<string, Buffer> {
  const buf = gunzipSync(readFileSync(path));
  const files = new Map<string, Buffer>();
  let off = 0;
  while (off + 512 <= buf.length) {
    const header = buf.subarray(off, off + 512);
    const name = header.subarray(0, 100).toString("utf8").replace(/\0.*$/s, "");
    if (name === "") break;
    const sizeText = header.subarray(124, 136).toString("utf8").replace(/\0.*$/s, "").trim();
    const size = sizeText === "" ? 0 : parseInt(sizeText, 8);
    const type = header.subarray(156, 157).toString("utf8");
    off += 512;
    if (type === "0" || type === "\0") {
      const inner = name.startsWith("package/") ? name.slice("package/".length) : name;
      files.set(inner, buf.subarray(off, off + size));
    }
    off += Math.ceil(size / 512) * 512;
  }
  return files;
}

function installedFiles(dir: string): string[] {
  const out: string[] = [];
  const walk = (d: string, rel: string) => {
    for (const name of readdirSync(d)) {
      const p = join(d, name);
      const r = rel === "" ? name : rel + "/" + name;
      if (statSync(p).isDirectory()) walk(p, r);
      else out.push(r);
    }
  };
  walk(dir, "");
  out.sort();
  return out;
}

function lock(): Record<string, any> {
  return JSON.parse(readFileSync("package-lock.json", "utf8"));
}

function manifest(): Record<string, any> {
  return JSON.parse(readFileSync("package.json", "utf8"));
}

test("plan-limits-are-si-bytes", () => {
  assert.equal(planBytes("team"), 2000000000);
  assert.equal(planBytes("enterprise"), 40000000000);
});

test("remaining-bytes-across-plans", () => {
  assert.equal(remainingBytes("750MB", "team"), 1250000000);
  assert.equal(remainingBytes("12GB", "enterprise"), 28000000000);
  assert.equal(remainingBytes("0.5GB", "team"), 1500000000);
});

test("binary-units-are-powers-of-1024", () => {
  assert.equal(parseBytes("64KiB"), 65536);
  assert.equal(parseBytes("1.5MiB"), 1572864);
  assert.equal(parseBytes("3GiB"), 3221225472);
  assert.equal(parseBytes("8TiB"), 8796093022208);
});

test("fits-plan-at-the-boundary", () => {
  assert.equal(fitsPlan("2GB", "team"), true);
  assert.equal(fitsPlan("2GiB", "team"), false);
  assert.equal(fitsPlan("2500B", "free"), true);
  assert.equal(fitsPlan("41GB", "enterprise"), false);
});

test("plan-limit-strings-unchanged", () => {
  assert.deepEqual(PLAN_LIMITS, { free: "50MB", team: "2GB", enterprise: "40GB" });
});

test("manifest-pins-the-new-tarball", () => {
  const pkg = manifest();
  const spec = String(pkg.dependencies["bytesize-parse"]);
  assert.ok(spec.includes("bytesize-parse-1.3.0.tgz"), spec);
  assert.ok(!spec.includes("1.2.0"), spec);
});

test("lockfile-records-the-vendored-tarball", () => {
  const l = lock();
  const entry = l.packages["node_modules/bytesize-parse"];
  assert.ok(entry !== undefined, "no lockfile entry for bytesize-parse");
  assert.equal(entry.version, "1.3.0");
  assert.ok(String(entry.resolved).includes("bytesize-parse-1.3.0.tgz"), String(entry.resolved));
  // The recorded hash must be the hash of the tarball that is actually vendored.
  assert.equal(entry.integrity, integrityOf(NEW_TARBALL));
  assert.notEqual(entry.integrity, integrityOf(OLD_TARBALL));
  // The root entry must agree with package.json.
  assert.equal(l.packages[""].dependencies["bytesize-parse"], manifest().dependencies["bytesize-parse"]);
});

test("installed-tree-is-the-tarball-unedited", () => {
  const want = tarFiles(NEW_TARBALL);
  assert.ok(want.has("index.js") && want.has("package.json"), [...want.keys()].join(","));
  assert.deepEqual(installedFiles(INSTALLED), [...want.keys()].sort());
  for (const [name, bytes] of want) {
    assert.equal(sha256(readFileSync(join(INSTALLED, name))), sha256(bytes), `${name} differs from the tarball`);
  }
  assert.equal(JSON.parse(readFileSync(join(INSTALLED, "package.json"), "utf8")).version, "1.3.0");
});

test("vendored-tarballs-unchanged", () => {
  assert.equal(sha256(readFileSync(OLD_TARBALL)), "sha256:6fd7c340e7aa2ed4b5833101b920118068966a11a9d683129081c64e44221409");
  assert.equal(sha256(readFileSync(NEW_TARBALL)), "sha256:7f10bdd7f238679fcdfeed427bd94a6891285b92fe6c3d127358d776d600a1b3");
});

test("install-is-only-the-vendored-package", () => {
  assert.deepEqual(Object.keys(manifest().dependencies), ["bytesize-parse"]);
  assert.deepEqual(Object.keys(lock().packages).sort(), ["", "node_modules/bytesize-parse"]);
  assert.deepEqual(readdirSync("node_modules").sort(), [".package-lock.json", "bytesize-parse"]);
});

test("unknown-unit-still-throws", () => {
  assert.throws(() => parseBytes("10 quatloos"), TypeError);
  assert.throws(() => parseBytes("plenty"), TypeError);
});

test("unknown-plan-still-throws", () => {
  assert.throws(() => planBytes("gold"), /gold/);
});
