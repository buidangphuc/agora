"use client";

import { useCallback, useRef, useState } from "react";

/**
 * Pending state for a Server Action call. `run` ignores a second call while the
 * first is in flight (exactly one request per double click) and clears `pending`
 * when it settles. Plain state rather than useTransition, so it behaves the same
 * with or without async-transition support.
 */
export function usePendingAction() {
  const [pending, setPending] = useState(false);
  const inFlight = useRef(false);

  const run = useCallback(
    async <T>(action: () => Promise<T>): Promise<T | undefined> => {
      if (inFlight.current) return undefined;
      inFlight.current = true;
      setPending(true);
      try {
        return await action();
      } finally {
        inFlight.current = false;
        setPending(false);
      }
    },
    [],
  );

  return { pending, run };
}
