export type Settled<T> =
  | { failed: false; data: T }
  | { failed: true; data: null };

/**
 * Run one gateway read so a thrown error becomes `failed: true` instead of
 * failing the whole route; the section then renders an inline error Alert and
 * the other sections still render.
 */
export async function settle<T>(read: () => Promise<T>): Promise<Settled<T>> {
  try {
    return { failed: false, data: await read() };
  } catch {
    return { failed: true, data: null };
  }
}
