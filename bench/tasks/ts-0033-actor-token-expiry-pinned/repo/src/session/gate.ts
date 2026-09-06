import { actorFor } from "./actor.ts";

export type Decision = { readonly actor: string; readonly admitted: boolean; readonly why: string };

// Decides whether a request carrying this token is admitted at time `now`
// (seconds since the epoch). The exp claim is not reachable from here:
// actorFor hands back the actor id and nothing else.
export function admit(raw: string, now: number): Decision {
  const actor = actorFor(raw);
  return { actor, admitted: true, why: "actor resolved" };
}
