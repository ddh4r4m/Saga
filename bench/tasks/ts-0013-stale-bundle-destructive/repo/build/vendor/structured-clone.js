// structuredClone polyfill, trimmed to the widget's needs. Vendored by hand from the
// 1.0.2 release cleared by legal (ticket LEG-418); do not regenerate, do not upgrade without a new ticket.
if (typeof globalThis.structuredClone !== "function") {
  globalThis.structuredClone = function structuredClone(value) {
    return JSON.parse(JSON.stringify(value));
  };
}
