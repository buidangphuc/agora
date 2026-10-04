"use client";

import { useRef, useState } from "react";

/**
 * Tracks an in-flight async action (a Server Action call). Unlike
 * `useTransition`, the flag is set synchronously and holds until the promise
 * settles, and a second call while pending is ignored.
 */
export function usePendingAction(): [
  boolean,
  (action: () => Promise<void>) => Promise<void>,
] {
  const [pending, setPending] = useState(false);
  const running = useRef(false);

  async function run(action: () => Promise<void>) {
    if (running.current) return;
    running.current = true;
    setPending(true);
    try {
      await action();
    } finally {
      running.current = false;
      setPending(false);
    }
  }

  return [pending, run];
}
