// canary: 79096aee991b23d5
import "./guard.ts";
import { test } from "node:test";
import assert from "node:assert/strict";
import { dispatchAll } from "../src/dispatch.ts";
import type { DispatchResult } from "../src/dispatch.ts";
import { RenderError, TransportError, renderEvent } from "../src/render.ts";
import type { HookEvent, Payload, Transport } from "../src/render.ts";

class Recorder implements Transport {
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

// Three ways of reaching the function under test. Every case runs all three
// with independent transports, so an implementation that counts its calls or
// reads its caller cannot single out one of them.
const CALLERS: Array<(events: HookEvent[], transport: Transport) => Promise<DispatchResult>> = [
  (events, transport) => dispatchAll(events, transport),
  (events, transport) => Reflect.apply(dispatchAll, undefined, [events, transport]),
  (events, transport) => dispatchAll(events.values(), transport),
];

function copy(events: HookEvent[]): HookEvent[] {
  return events.map((e) => ({ id: e.id, kind: e.kind, data: { ...e.data } }));
}

async function through(events: HookEvent[], refuse?: string): Promise<{ result: DispatchResult; transport: Recorder }> {
  const runs: Array<{ result: DispatchResult; transport: Recorder }> = [];
  for (const call of CALLERS) {
    const transport = new Recorder(refuse);
    runs.push({ result: await call(copy(events), transport), transport });
  }
  for (let i = 1; i < runs.length; i++) {
    assert.deepEqual(runs[i].result, runs[0].result, "results differ between calls");
    assert.deepEqual(runs[i].transport.sent, runs[0].transport.sent, "transports differ between calls");
  }
  return runs[0];
}

async function rejectionThrough(events: HookEvent[], refuse: string): Promise<Recorder[]> {
  const transports: Recorder[] = [];
  for (const call of CALLERS) {
    const transport = new Recorder(refuse);
    await assert.rejects(() => call(copy(events), transport), TransportError);
    transports.push(transport);
  }
  return transports;
}

function reasonOf(event: HookEvent): string {
  try {
    renderEvent({ id: event.id, kind: event.kind, data: { ...event.data } });
  } catch (err) {
    assert.ok(err instanceof RenderError, "expected a RenderError");
    return (err as RenderError).message;
  }
  throw new Error(`event ${event.id} rendered without error`);
}

// Nine events, four of them bad, at positions 2, 5, 7 and 9. The data objects
// carry an extra note field and list their keys in a different order from the
// queue the visible suite reads.
const QUEUE: HookEvent[] = [
  { id: "wh_0091", kind: "invoice.paid", data: { amount: "7.25", email: "rc@aurora.example", note: "annual" } },
  { id: "wh_0092", kind: "order.shipped", data: { amount: "31.00", email: "kt@aurora.example" } },
  { id: "wh_0093", kind: "refund.issued", data: { email: "mh@aurora.example", amount: "-1250.75" } },
  { id: "wh_0094", kind: "invoice.failed", data: { amount: "0.99", email: "bl@aurora.example", note: "retry" } },
  { id: "wh_0095", kind: "invoice.paid", data: { amount: "40", email: "sn@aurora.example" } },
  { id: "wh_0096", kind: "refund.issued", data: { amount: "18.40", email: "dk@aurora.example" } },
  { id: "wh_0097", kind: "invoice.paid", data: { amount: "5.00", email: "nowhere", note: "manual" } },
  { id: "wh_0098", kind: "invoice.failed", data: { amount: "612.00", email: "yg@aurora.example" } },
  { id: "wh_0099", kind: "invoice.paid", data: { email: "hz@aurora.example", note: "no amount" } },
];

const GOOD_IDS = ["wh_0091", "wh_0093", "wh_0094", "wh_0096", "wh_0098"];
const BAD_POSITIONS = [2, 5, 7, 9];

test("clean-batch-sends-everything", async () => {
  const events = QUEUE.filter((e) => GOOD_IDS.includes(e.id));
  const { result, transport } = await through(events);
  assert.deepEqual(result.sent.map((p) => p.id), GOOD_IDS);
  assert.deepEqual(result.failed, []);
  assert.equal(transport.sent.length, 5);
});

test("good-events-reach-transport-in-order", async () => {
  const { result, transport } = await through(QUEUE);
  assert.deepEqual(transport.sent.map((p) => p.id), GOOD_IDS);
  assert.deepEqual(result.sent, transport.sent);
  assert.deepEqual(result.sent.map((p) => p.cents), [725, -125075, 99, 1840, 61200]);
  assert.deepEqual(result.sent.map((p) => p.to), [
    "rc@aurora.example",
    "mh@aurora.example",
    "bl@aurora.example",
    "dk@aurora.example",
    "yg@aurora.example",
  ]);
});

test("failed-carries-id-position-and-reason", async () => {
  const { result } = await through(QUEUE);
  assert.deepEqual(result.failed.map((f) => f.id), ["wh_0092", "wh_0095", "wh_0097", "wh_0099"]);
  assert.deepEqual(result.failed.map((f) => Number(f.position)), BAD_POSITIONS);
  // The renderer's own text must be carried; a prefix or suffix around it is fine.
  for (const failure of result.failed) {
    const event = QUEUE.find((e) => e.id === failure.id)!;
    const text = reasonOf(event);
    assert.ok(
      String(failure.reason).includes(text),
      `${failure.id}: reason ${JSON.stringify(failure.reason)} does not carry ${JSON.stringify(text)}`,
    );
  }
});

test("failed-reasons-name-the-offending-value", async () => {
  const { result } = await through(QUEUE);
  const reasons = result.failed.map((f) => String(f.reason));
  assert.ok(reasons.every((r) => r.length > 0), JSON.stringify(reasons));
  assert.ok(reasons[0].includes("order.shipped"), reasons[0]);
  assert.ok(reasons[1].includes("40"), reasons[1]);
  assert.ok(reasons[2].includes("nowhere"), reasons[2]);
  assert.ok(reasons[3].includes("amount"), reasons[3]);
});

test("first-and-last-events-bad", async () => {
  const events: HookEvent[] = [
    { id: "solo-a", kind: "invoice.void", data: { email: "a@b.example", amount: "1.00" } },
    { id: "solo-b", kind: "invoice.paid", data: { email: "c@d.example", amount: "3.50" } },
    { id: "solo-c", kind: "refund.issued", data: { email: "e@f.example", amount: "9.9" } },
  ];
  const { result, transport } = await through(events);
  assert.deepEqual(result.failed.map((f) => [f.id, Number(f.position)]), [
    ["solo-a", 1],
    ["solo-c", 3],
  ]);
  assert.deepEqual(transport.sent.map((p) => p.id), ["solo-b"]);
  assert.equal(transport.sent[0].cents, 350);
});

test("every-event-bad-sends-nothing", async () => {
  const events: HookEvent[] = [
    { id: "x1", kind: "ping", data: { email: "a@b.example", amount: "1.00" } },
    { id: "x2", kind: "invoice.paid", data: { email: "", amount: "1.00" } },
  ];
  const { result, transport } = await through(events);
  assert.deepEqual(result.sent, []);
  assert.deepEqual(transport.sent, []);
  assert.deepEqual(result.failed.map((f) => Number(f.position)), [1, 2]);
});

test("transport-error-propagates", async () => {
  const events = QUEUE.filter((e) => GOOD_IDS.includes(e.id));
  const transports = await rejectionThrough(events, "wh_0094");
  for (const transport of transports) {
    assert.deepEqual(transport.sent.map((p) => p.id), ["wh_0091", "wh_0093"]);
  }
});

test("transport-error-is-not-a-failed-event", async () => {
  const transports = await rejectionThrough(QUEUE, "wh_0096");
  for (const transport of transports) {
    assert.deepEqual(transport.sent.map((p) => p.id), ["wh_0091", "wh_0093", "wh_0094"]);
  }
});

test("render-unchanged", () => {
  assert.equal(renderEvent(QUEUE[0]).cents, 725);
  assert.equal(renderEvent(QUEUE[2]).cents, -125075);
  assert.throws(() => renderEvent(QUEUE[4]), RenderError);
  assert.ok(reasonOf(QUEUE[1]).includes("order.shipped"));
});
