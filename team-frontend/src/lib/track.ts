/**
 * Browser-only behavioral tracking beacon. Fires a lightweight, fire-and-forget
 * beacon to the Gateway edge collector (POST /api/track) on view / click /
 * add-to-cart / search-impression. It is best-effort telemetry (ARCHITECTURE
 * Rule 1: the frontend talks only to the gateway): a dropped beacon must never
 * block or fail the user's browsing action, so nothing here ever throws into the
 * caller.
 *
 * The payload carries behavioral context ONLY — no PII. Authenticated identity
 * is attached at the edge via the EventEnvelope principal (the session cookie is
 * sent automatically). We only ship an anonymous id + session id (cookie/device
 * scoped) so downstream can group a visitor's activity.
 */

export type TrackEventType = "view" | "click" | "add_to_cart" | "impression";

/** A single tracked browsing action. Behavioral context only — never PII. */
export interface TrackEvent {
  readonly type: TrackEventType;
  readonly listingId?: string;
  /** Page path; defaults to the current location pathname. */
  readonly path?: string;
  /** Referrer; defaults to document.referrer. */
  readonly referrer?: string;
  /** 1-based rank within a result set (search impressions/clicks). */
  readonly position?: number;
  /** Search query in effect, when applicable. */
  readonly query?: string;
  /** Open-ended extension bag for experimental attributes. */
  readonly properties?: Record<string, string>;
}

/** The JSON shape POSTed to the gateway collector. */
export interface TrackBeacon {
  readonly type: TrackEventType;
  readonly listingId: string;
  readonly sessionId: string;
  readonly anonymousId: string;
  readonly path: string;
  readonly referrer: string;
  readonly position: number;
  readonly query: string;
  readonly properties?: Record<string, string>;
}

const ANONYMOUS_ID_KEY = "bds_anonymous_id";
const SESSION_ID_KEY = "bds_session_id";

function gatewayUrl(): string {
  const url = process.env.NEXT_PUBLIC_GATEWAY_URL?.trim();
  return url && url.length > 0 ? url : "http://localhost:8080";
}

function randomId(): string {
  try {
    if (typeof crypto !== "undefined" && crypto.randomUUID) {
      return crypto.randomUUID();
    }
  } catch {
    // fall through to the non-crypto id below
  }
  return `id-${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`;
}

/**
 * Read-or-create a stable id in the given Storage. Returns "" if storage is
 * unavailable (private mode, disabled cookies) — the gateway tolerates an empty
 * id, so tracking silently degrades instead of breaking.
 */
function persistentId(storage: Storage | undefined, key: string): string {
  if (!storage) return "";
  try {
    const existing = storage.getItem(key);
    if (existing) return existing;
    const id = randomId();
    storage.setItem(key, id);
    return id;
  } catch {
    return "";
  }
}

function safeStorage(kind: "local" | "session"): Storage | undefined {
  try {
    return kind === "local" ? window.localStorage : window.sessionStorage;
  } catch {
    return undefined;
  }
}

/**
 * Build the beacon payload for an event. Pure aside from reading the persisted
 * anonymous/session ids and the current location — exported for testing.
 */
export function buildBeacon(event: TrackEvent): TrackBeacon {
  const anonymousId = persistentId(safeStorage("local"), ANONYMOUS_ID_KEY);
  const sessionId = persistentId(safeStorage("session"), SESSION_ID_KEY);
  const path =
    event.path ??
    (typeof location !== "undefined" ? location.pathname : "") ??
    "";
  const referrer =
    event.referrer ??
    (typeof document !== "undefined" ? document.referrer : "") ??
    "";
  return {
    type: event.type,
    listingId: event.listingId ?? "",
    sessionId,
    anonymousId,
    path,
    referrer,
    position: event.position ?? 0,
    query: event.query ?? "",
    ...(event.properties ? { properties: event.properties } : {}),
  };
}

/**
 * Send the beacon. Prefers navigator.sendBeacon (survives page unload); falls
 * back to a keepalive fetch. Sent as text/plain so the browser treats it as a
 * CORS-simple request (no preflight) — the gateway reads the raw JSON body
 * regardless of content-type. Never throws.
 */
function send(beacon: TrackBeacon): void {
  const url = `${gatewayUrl().replace(/\/$/, "")}/api/track`;
  const body = JSON.stringify(beacon);
  try {
    if (typeof navigator !== "undefined" && navigator.sendBeacon) {
      const blob = new Blob([body], { type: "text/plain;charset=UTF-8" });
      if (navigator.sendBeacon(url, blob)) return;
    }
  } catch {
    // fall through to fetch
  }
  try {
    void fetch(url, {
      method: "POST",
      keepalive: true,
      credentials: "include",
      headers: { "Content-Type": "text/plain;charset=UTF-8" },
      body,
    }).catch(() => {
      // best-effort: swallow network errors
    });
  } catch {
    // never throw into the caller
  }
}

/**
 * Fire a tracking beacon for a browsing action. Safe to call from anywhere in
 * the browser; a no-op during SSR. Never throws.
 */
export function track(event: TrackEvent): void {
  if (typeof window === "undefined") return;
  try {
    send(buildBeacon(event));
  } catch {
    // telemetry must never break browsing
  }
}
