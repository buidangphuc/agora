/**
 * Admin cockpit data over the Gateway's GET /api/admin/metrics (the frontend's
 * only outbound path, Rule 1). server-only: the session bearer is attached here
 * and never reaches the browser. The Gateway is the enforcement point (401 with
 * no token, 403 without the `admin` scope); the page's own scope check only
 * decides what to render.
 */
import "server-only";

import type { CockpitData } from "@/features/admin/CockpitView";

import { gatewayConfig } from "./config.js";

export type CockpitResult =
  | { status: "ok"; data: CockpitData }
  | { status: "unauthenticated" }
  | { status: "forbidden" }
  | { status: "unavailable" };

export type {
  TrackingQuality,
  TrackingQualityType,
} from "@/features/admin/TrackingQualityPanel";

const TIMEOUT_MS = 5000;

/** Fetch the cockpit payload with `token`; never throws. */
export async function fetchCockpit(token: string): Promise<CockpitResult> {
  try {
    const res = await fetch(`${gatewayConfig.gatewayUrl}/api/admin/metrics`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
    if (res.status === 401) return { status: "unauthenticated" };
    if (res.status === 403) return { status: "forbidden" };
    if (!res.ok) return { status: "unavailable" };
    return { status: "ok", data: (await res.json()) as CockpitData };
  } catch {
    return { status: "unavailable" };
  }
}
