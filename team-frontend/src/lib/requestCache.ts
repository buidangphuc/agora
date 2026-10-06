import * as React from "react";

type Fn<A extends unknown[], R> = (...args: A) => R;

// React's request-scoped cache() ships with the RSC runtime Next bundles but not
// with plain React (Vitest, jsdom), so fall back to the uncached function there.
const reactCache = (
  React as unknown as {
    cache?: <A extends unknown[], R>(fn: Fn<A, R>) => Fn<A, R>;
  }
).cache;

/**
 * Memoise a server loader for the duration of one request, so a segment layout
 * (which decides the HTTP status via notFound()) and its page share one gateway
 * call instead of fetching twice.
 */
export function requestCache<A extends unknown[], R>(fn: Fn<A, R>): Fn<A, R> {
  return reactCache ? reactCache(fn) : fn;
}
