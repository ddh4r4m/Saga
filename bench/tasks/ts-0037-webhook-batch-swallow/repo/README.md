# hookrelay

Relays billing events to customer webhook endpoints. `dispatchAll(events, transport)` in `src/dispatch.ts` renders each queued event into a payload and hands it to the transport; the result lists what was sent and what failed.

An event is `{ id, kind, data }`; `renderEvent` in `src/render.ts` turns it into a payload or throws `RenderError` with the reason. The transport raises `TransportError` when the endpoint refuses a payload.

Tests: `npm test`
