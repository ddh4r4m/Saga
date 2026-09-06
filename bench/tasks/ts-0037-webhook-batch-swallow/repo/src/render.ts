// Rendering one queued event into the payload the endpoint expects.

export class RenderError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "RenderError";
  }
}

export class TransportError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "TransportError";
  }
}

export interface HookEvent {
  id: string;
  kind: string;
  data: Record<string, string>;
}

export interface Payload {
  id: string;
  kind: string;
  to: string;
  cents: number;
}

export interface Transport {
  // Delivers one payload; throws TransportError when the endpoint refuses it.
  send(payload: Payload): Promise<void>;
}

const KINDS = ["invoice.paid", "invoice.failed", "refund.issued"];

export function renderEvent(event: HookEvent): Payload {
  if (!KINDS.includes(event.kind)) {
    throw new RenderError(`unknown kind '${event.kind}'`);
  }
  const to = event.data.email;
  if (to === undefined || to === "") {
    throw new RenderError("missing field 'email'");
  }
  if (!to.includes("@")) {
    throw new RenderError(`bad email '${to}'`);
  }
  const amount = event.data.amount;
  if (amount === undefined || amount === "") {
    throw new RenderError("missing field 'amount'");
  }
  if (!/^-?\d+\.\d{2}$/.test(amount)) {
    throw new RenderError(`bad amount '${amount}'`);
  }
  const negative = amount.startsWith("-");
  const [whole, fraction] = (negative ? amount.slice(1) : amount).split(".");
  const cents = Number(whole) * 100 + Number(fraction);
  return { id: event.id, kind: event.kind, to, cents: negative ? -cents : cents };
}
