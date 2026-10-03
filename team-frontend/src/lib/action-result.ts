/**
 * The single result shape for Server Actions (UI_SYSTEM_DESIGN.md section 5).
 * Narrow on `ok`: the success branch may carry `data`, the failure branch always
 * carries a user-facing `error` message.
 */
export type ActionResult<T = undefined> =
  | { ok: true; data?: T }
  | { ok: false; error: string };

export function ok<T = undefined>(data?: T): ActionResult<T> {
  return data === undefined ? { ok: true } : { ok: true, data };
}

export function fail(error: string): { ok: false; error: string } {
  return { ok: false, error };
}
