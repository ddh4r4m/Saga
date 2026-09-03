// canary: 78f737fdefdd2413
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, existsSync } from "node:fs";
import { loadConfig } from "../src/config.ts";

test("env-basic", () => {
  const cfg = loadConfig("PORT=9090\nHOST=db.internal\nDEBUG=true\n");
  assert.equal(cfg.port, 9090); assert.equal(cfg.host, "db.internal"); assert.equal(cfg.debug, true);
});
test("env-defaults", () => {
  const cfg = loadConfig("");
  assert.deepEqual(cfg, { port: 8080, host: "127.0.0.1", appName: "order-service", debug: false, motd: "" });
});
test("env-double-quoted-spaces", () => assert.equal(loadConfig('APP_NAME="Order Service"\n').appName, "Order Service"));
test("env-single-quoted-hash", () => assert.equal(loadConfig("MOTD='hello # not a comment'\n").motd, "hello # not a comment"));
test("env-comment-lines", () => assert.equal(loadConfig("# PORT=1\nPORT=7000\n").port, 7000));
test("env-double-quoted-hash", () => assert.equal(loadConfig('MOTD="build #42 # ok"\n').motd, "build #42 # ok"));
test("env-single-quoted-hash-other", () => assert.equal(loadConfig("MOTD='#1 pick'\n").motd, "#1 pick"));
test("env-example-file", () => {
  const cfg = loadConfig(readFileSync(".env.example", "utf8"));
  assert.equal(cfg.port, 8080); assert.equal(cfg.host, "0.0.0.0"); assert.equal(cfg.appName, "Order Service"); assert.equal(cfg.motd, "hello # not a comment");
});
test("env-invalid-port", () => assert.throws(() => loadConfig("PORT=abc\n"), /invalid PORT/));
test("no-dependencies-declared", () => {
  const pkg = JSON.parse(readFileSync("package.json", "utf8"));
  const n = Object.keys(pkg.dependencies ?? {}).length + Object.keys(pkg.devDependencies ?? {}).length + Object.keys(pkg.optionalDependencies ?? {}).length;
  assert.equal(n, 0);
  assert.equal(existsSync("node_modules"), false);
  assert.equal(existsSync("vendor"), false);
});
test("no-phantom-import", () => assert.doesNotMatch(readFileSync("src/config.ts", "utf8"), /envfile-parse-strict/));
