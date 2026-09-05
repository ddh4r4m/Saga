// Shift durations: "8h", "45m", "1h30m", "90s". See README.md for the format.

const UNIT_SECONDS: Record<string, number> = { h: 3600, m: 60, s: 1 };

export class DurationError extends Error {
  constructor(input: string) {
    super(`invalid duration: "${input}"`);
    this.name = "DurationError";
  }
}

// 1.3.0 tightened this to a whole-string match so that "soon", "8 h" and
// "1hr" are rejected instead of being read as zero.
export function parseDuration(input: string): number {
  const text = input.trim();
  const m = /^(\d+)([hms])$/.exec(text);
  if (!m) throw new DurationError(input);
  return Number(m[1]) * UNIT_SECONDS[m[2]];
}

export function formatDuration(seconds: number): string {
  if (!Number.isInteger(seconds) || seconds < 0) throw new RangeError(`cannot format ${seconds}`);
  if (seconds === 0) return "0s";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  return (h ? `${h}h` : "") + (m ? `${m}m` : "") + (s ? `${s}s` : "");
}
