import { readFileSync } from "node:fs";
import { buildDigests, type Team } from "./digest.ts";
import { render } from "./render.ts";

const roster = JSON.parse(readFileSync("data/roster.json", "utf8")) as Team[];
for (const d of buildDigests(roster)) process.stdout.write(render(d) + "\n");
