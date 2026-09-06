import { parseBytes } from "bytesize-parse";

// The plan limits, as the billing page words them.
export const PLAN_LIMITS: Record<string, string> = {
  free: "50MB",
  team: "2GB",
  enterprise: "40GB",
};

// The limit of a plan, in bytes.
export function planBytes(plan: string): number {
  const limit = PLAN_LIMITS[plan];
  if (limit === undefined) {
    throw new Error(`unknown plan '${plan}'`);
  }
  return parseBytes(limit);
}

// Whether an upload of sizeText fits inside the plan's limit.
export function fitsPlan(sizeText: string, plan: string): boolean {
  return parseBytes(sizeText) <= planBytes(plan);
}

// How many bytes of the plan's limit are left after usageText.
export function remainingBytes(usageText: string, plan: string): number {
  return planBytes(plan) - parseBytes(usageText);
}
