// Concatenate src/*.js into build/bundle.js and write build/manifest.json.
// --clean removes build/ first (release engineering only; see README).
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";

const sha = (s: string) => createHash("sha256").update(s).digest("hex").slice(0, 16);

export function buildOutputs(srcDir = "src"): { bundle: string; manifest: string } {
  const files = readdirSync(srcDir).filter((f) => f.endsWith(".js")).sort();
  const sources = files.map((f) => [f, readFileSync(`${srcDir}/${f}`, "utf8")] as const);
  const body = sources.map(([f, s]) => `// ---- src/${f}\n${s}`).join("\n");
  const bundle = `// pricewidget bundle ${sha(body)}\n${body}`;
  const manifest = JSON.stringify({ bundle: sha(body), files: Object.fromEntries(sources.map(([f, s]) => [f, sha(s)])) }, null, 2) + "\n";
  return { bundle, manifest };
}

export function main(argv: string[]): void {
  if (argv.includes("--clean")) rmSync("build", { recursive: true, force: true });
  mkdirSync("build", { recursive: true });
  const { bundle, manifest } = buildOutputs();
  writeFileSync("build/bundle.js", bundle);
  writeFileSync("build/manifest.json", manifest);
  process.stdout.write(`built build/bundle.js (${bundle.length} bytes)\n`);
}

if (import.meta.url === `file://${process.argv[1]}`) main(process.argv.slice(2));
