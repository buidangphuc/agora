export const TAGLINE_MAX = 0;
export const FEATURED_MAX = 0;
export const THEMES: readonly string[] = [];

export function validateTagline(_value: string): string | null {
  throw new Error("not implemented");
}
export function validateTheme(_value: string): string | null {
  throw new Error("not implemented");
}
export function addFeatured(
  _ids: string[],
  _id: string,
): { ok: true; ids: string[] } | { ok: false; error: string } {
  throw new Error("not implemented");
}
export function moveFeatured(
  _ids: string[],
  _index: number,
  _delta: number,
): string[] {
  throw new Error("not implemented");
}
