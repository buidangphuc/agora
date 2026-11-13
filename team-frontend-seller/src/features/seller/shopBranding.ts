export const TAGLINE_MAX = 120;
export const FEATURED_MAX = 6;
export const THEMES: readonly string[] = ["default", "dark", "festive"];

function isControl(ch: string): boolean {
  const c = ch.codePointAt(0) ?? 0;
  return c <= 0x1f || (c >= 0x7f && c <= 0x9f);
}

export function validateTagline(value: string): string | null {
  const v = value.trim();
  if ([...v].some(isControl)) {
    return "Tagline không được chứa ký tự điều khiển.";
  }
  if ([...v].length > TAGLINE_MAX) return "Tagline tối đa 120 ký tự.";
  return null;
}
export function validateTheme(value: string): string | null {
  return THEMES.includes(value) ? null : "Giao diện không hợp lệ.";
}
export function addFeatured(
  ids: string[],
  id: string,
): { ok: true; ids: string[] } | { ok: false; error: string } {
  if (ids.includes(id)) return { ok: true, ids: [...ids] };
  if (ids.length >= FEATURED_MAX) {
    return { ok: false, error: "Tối đa 6 sản phẩm nổi bật." };
  }
  return { ok: true, ids: [...ids, id] };
}
export function moveFeatured(
  ids: string[],
  index: number,
  delta: number,
): string[] {
  const to = index + delta;
  if (index < 0 || index >= ids.length || to < 0 || to >= ids.length) {
    return [...ids];
  }
  const out = [...ids];
  [out[index], out[to]] = [out[to], out[index]];
  return out;
}
