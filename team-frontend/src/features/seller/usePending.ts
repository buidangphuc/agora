"use client";

import { useCallback, useState } from "react";

/**
 * Tracks a Server Action call: `pending` is true from the call until it
 * settles. Plain state instead of useTransition, whose isPending does not
 * follow an async callback on every React version.
 */
export function usePending() {
  const [pending, setPending] = useState(false);
  const run = useCallback(async <T>(fn: () => Promise<T>): Promise<T> => {
    setPending(true);
    try {
      return await fn();
    } finally {
      setPending(false);
    }
  }, []);
  return { pending, run };
}
