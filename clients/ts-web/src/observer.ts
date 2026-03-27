/**
 * observer.ts — typed helpers for interpreting state snapshots received from
 * the game-engine server.
 *
 * NOTE: Real-time observation via gRPC streaming requires an Envoy gRPC-Web
 * proxy (or grpc-web compatible gateway) in front of the game-engine server.
 * The browser cannot speak raw HTTP/2 gRPC directly; all streaming must be
 * mediated by the proxy.
 *
 * This file intentionally contains no network I/O.  It re-exports the
 * StateUpdate interface from the generated proto types and provides a utility
 * for decoding opaque state-snapshot payloads.
 */

// Re-export the generated proto types so consumers can import from one place.
export type { State, StateUpdate } from "./proto/common";

/**
 * Attempt to decode a state snapshot payload.
 *
 * If the payload is a `string` it is treated as a JSON string and parsed
 * directly.  If it is a `Uint8Array` the bytes are decoded as UTF-8 and then
 * parsed as JSON.  Any parse failures result in the raw value being wrapped in
 * an object under the key `"raw"`.
 *
 * @param payload - Raw bytes or JSON string from a ReplayEntry.stateSnapshot
 *                  or State.payload field.
 * @returns A plain JavaScript object representing the decoded state.
 */
export function parseStateSnapshot(
  payload: Uint8Array | string,
): Record<string, unknown> {
  let jsonText: string;

  if (typeof payload === "string") {
    jsonText = payload;
  } else {
    jsonText = new TextDecoder("utf-8").decode(payload);
  }

  if (jsonText.trim().length === 0) {
    return {};
  }

  try {
    return JSON.parse(jsonText) as Record<string, unknown>;
  } catch {
    return { raw: jsonText };
  }
}
