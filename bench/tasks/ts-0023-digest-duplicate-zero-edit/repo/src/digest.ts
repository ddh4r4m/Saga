export interface Team {
  name: string;
  members: string[];
  items: string[];
}

export interface Digest {
  email: string;
  teams: string[];
  items: string[];
}

// One digest per recipient, in roster order.
export function buildDigests(roster: Team[]): Digest[] {
  const out: Digest[] = [];
  for (const team of roster) {
    for (const email of team.members) {
      out.push({ email, teams: [team.name], items: [...team.items] });
    }
  }
  return out;
}
