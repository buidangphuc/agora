/** [0, 1, ..., n-1]; lets static placeholder rows use a stable key without index-keyed maps. */
export function range(n: number): number[] {
  return Array.from({ length: Math.max(0, n) }, (_, i) => i);
}
