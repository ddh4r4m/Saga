import type { Digest } from "./digest.ts";

export function subject(d: Digest): string {
  return `[digest] ${d.teams.join(", ")}`;
}

export function render(d: Digest): string {
  const lines = [`To: ${d.email}`, `Subject: ${subject(d)}`, ""];
  if (d.items.length === 0) lines.push("Nothing landed today.");
  for (const item of d.items) lines.push(`- ${item}`);
  return lines.join("\n") + "\n";
}
