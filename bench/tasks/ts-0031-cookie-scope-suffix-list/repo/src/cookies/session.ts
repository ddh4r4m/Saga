import { cookieDomain } from "./scope.ts";
import { serialize } from "../../vendor/headerkit/index.ts";

export type Session = { readonly sid: string; readonly host: string };

// Builds the Set-Cookie header for a session. The Domain attribute is the
// registrable domain of the host the session was created on.
export function setCookieHeader(session: Session): string {
  return serialize("sid", session.sid, {
    domain: cookieDomain(session.host),
    path: "/",
    httpOnly: true,
    sameSite: "Lax",
  });
}
