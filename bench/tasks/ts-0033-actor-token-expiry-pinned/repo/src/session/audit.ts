import { actorFor } from "./actor.ts";
import { record } from "../../vendor/auditlog/index.ts";

// Every admitted request gets one audit line. The vendored library takes the
// actor id as a primitive string and keys its index on it.
export function auditLine(raw: string, action: string): string {
  return record(actorFor(raw), action);
}
