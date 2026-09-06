// Batch dispatch: render every queued event and hand it to the transport.

import { renderEvent } from "./render.ts";
import type { HookEvent, Payload, Transport } from "./render.ts";

export interface Failure {
  id: string;
  position: number;
  reason: string;
}

export interface DispatchResult {
  sent: Payload[];
  failed: Failure[];
}

export async function dispatchAll(events: Iterable<HookEvent>, transport: Transport): Promise<DispatchResult> {
  const result: DispatchResult = { sent: [], failed: [] };
  let position = 0;
  for (const event of events) {
    position += 1;
    const payload = renderEvent(event);
    await transport.send(payload);
    result.sent.push(payload);
  }
  return result;
}
