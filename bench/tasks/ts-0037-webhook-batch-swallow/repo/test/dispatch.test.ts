import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dispatchAll } from "../src/dispatch.ts";
import { TransportError } from "../src/render.ts";
import type { HookEvent, Payload } from "../src/render.ts";

class ListTransport {
  sent: Payload[] = [];
  refuse: string | undefined;
  constructor(refuse?: string) {
    this.refuse = refuse;
  }
  async send(payload: Payload): Promise<void> {
    if (this.refuse !== undefined && payload.id === this.refuse) {
      throw new TransportError(`endpoint refused ${payload.id}`);
    }
    this.sent.push(payload);
  }
}

function queue(): HookEvent[] {
  return JSON.parse(readFileSync("fixtures/queue.json", "utf8")) as HookEvent[];
}

test("good events are dispatched", async () => {
  const transport = new ListTransport();
  const result = await dispatchAll(queue().slice(0, 2), transport);
  assert.equal(result.sent.length, 2);
  assert.equal(transport.sent[1].cents, -90000);
});

test("a bad event does not stop the batch", async () => {
  const transport = new ListTransport();
  const result = await dispatchAll(queue(), transport);
  assert.deepEqual(result.sent.map((p) => p.id), ["evt-1", "evt-2", "evt-4"]);
  assert.equal(transport.sent.length, 3);
});
