"use client";

import { useRouter } from "next/navigation";
import { startTransition } from "react";

/**
 * Retry handler for route error boundaries. `reset()` alone only re-renders
 * the cached (failed) segment; `router.refresh()` first invalidates the router
 * cache and re-fetches the server components, so a recovered backend shows up.
 */
export function useRouteRetry(reset: () => void): () => void {
  const router = useRouter();
  return () => {
    startTransition(() => {
      router.refresh();
      reset();
    });
  };
}
