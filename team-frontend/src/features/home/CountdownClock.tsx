"use client";

import { useEffect, useState } from "react";

/** The clock's box: fixed width + tabular digits, so a tick never moves layout. */
const CLOCK_CLASS =
  "inline-block min-w-24 rounded-lg bg-neutral-900 px-2 py-0.5 text-center text-sm font-semibold tabular-nums text-text-inverse";

const PLACEHOLDER = "--:--:--";

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/** hh:mm:ss until `endsAtMs` (never negative). */
export function formatRemaining(endsAtMs: number, nowMs: number): string {
  const total = Math.max(0, Math.floor((endsAtMs - nowMs) / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return `${pad(h)}:${pad(m)}:${pad(s)}`;
}

/**
 * Ticking countdown to a REAL campaign end. It is the only client code of
 * FlashSaleSection and is mounted only when an `endsAt` exists. The server
 * render and first client render both show a same-width placeholder, so
 * hydration never mismatches and nothing shifts when the first tick lands.
 */
export function CountdownClock({ endsAt }: { endsAt: string | number }) {
  const endsAtMs = typeof endsAt === "number" ? endsAt : Date.parse(endsAt);
  const [text, setText] = useState(PLACEHOLDER);

  useEffect(() => {
    if (Number.isNaN(endsAtMs)) return;
    const tick = () => setText(formatRemaining(endsAtMs, Date.now()));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [endsAtMs]);

  if (Number.isNaN(endsAtMs)) return null;

  return (
    <time
      data-testid="countdown-clock"
      dateTime={new Date(endsAtMs).toISOString()}
      className={CLOCK_CLASS}
      aria-label="Thời gian còn lại"
    >
      {text}
    </time>
  );
}
