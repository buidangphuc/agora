import { WireTrackBeacon } from "./schema";

function gatewayUrl(): string {
  const url = process.env.NEXT_PUBLIC_GATEWAY_URL?.trim();
  return url && url.length > 0 ? url : "http://localhost:8080";
}

class BeaconQueue {
  private buffer: WireTrackBeacon[] = [];
  private flushTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly maxBatchSize = 20;
  private readonly flushIntervalMs = 2000;

  constructor() {
    if (typeof window !== "undefined") {
      window.addEventListener("visibilitychange", () => {
        if (document.visibilityState === "hidden") {
          this.flush();
        }
      });
      window.addEventListener("beforeunload", () => {
        this.flush();
      });
    }
  }

  public enqueue(beacon: WireTrackBeacon): void {
    this.buffer.push(beacon);
    if (this.buffer.length >= this.maxBatchSize) {
      this.flush();
    } else {
      this.scheduleFlush();
    }
  }

  public enqueueBatch(beacons: WireTrackBeacon[]): void {
    if (beacons.length === 0) return;
    this.buffer.push(...beacons);
    if (this.buffer.length >= this.maxBatchSize) {
      this.flush();
    } else {
      this.scheduleFlush();
    }
  }

  private scheduleFlush(): void {
    if (this.flushTimer !== null) return;
    this.flushTimer = setTimeout(() => {
      this.flush();
    }, this.flushIntervalMs);
  }

  public flush(): void {
    if (this.flushTimer !== null) {
      clearTimeout(this.flushTimer);
      this.flushTimer = null;
    }
    if (this.buffer.length === 0) return;

    const batch = this.buffer.splice(0, this.buffer.length);
    this.sendBatch(batch);
  }

  private sendBatch(batch: WireTrackBeacon[]): void {
    const url = `${gatewayUrl().replace(/\/$/, "")}/api/track`;
    const body = JSON.stringify(batch);

    try {
      if (typeof navigator !== "undefined" && navigator.sendBeacon) {
        const blob = new Blob([body], { type: "text/plain;charset=UTF-8" });
        if (navigator.sendBeacon(url, blob)) return;
      }
    } catch {
      // Fall through to fetch
    }

    try {
      void fetch(url, {
        method: "POST",
        keepalive: true,
        credentials: "include",
        headers: { "Content-Type": "text/plain;charset=UTF-8" },
        body,
      }).catch(() => {
        // Best-effort: swallow network errors
      });
    } catch {
      // Never throw into caller
    }
  }
}

export const beaconQueue = new BeaconQueue();
