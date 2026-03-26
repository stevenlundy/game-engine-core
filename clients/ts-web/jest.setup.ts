// jest.setup.ts — runs before each test suite
// Polyfill TextEncoder / TextDecoder for jsdom (not included by default).
import { TextEncoder, TextDecoder } from "util";

if (typeof global.TextEncoder === "undefined") {
  global.TextEncoder = TextEncoder as unknown as typeof global.TextEncoder;
}
if (typeof global.TextDecoder === "undefined") {
  global.TextDecoder = TextDecoder as unknown as typeof global.TextDecoder;
}

// Provide a stub fetch so jest.spyOn(global, 'fetch') has something to latch on to.
if (typeof global.fetch === "undefined") {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (global as any).fetch = () => Promise.reject(new Error("fetch not mocked"));
}
