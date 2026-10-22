export function parseItemsParam(raw: string | undefined): string[] {
  if (!raw) return [];
  const seen = new Set<string>();
  for (const part of raw.split(",")) {
    const id = part.trim();
    if (id !== "") seen.add(id);
  }
  return [...seen];
}

export function resolveSelection(
  cartLineIds: string[],
  raw: string | undefined,
): string[] {
  const wanted = new Set(parseItemsParam(raw));
  return cartLineIds.filter((id) => wanted.has(id));
}

export function selectionHref(
  ids: string[],
  extra: Record<string, string> = {},
): string {
  const q = new URLSearchParams({ items: ids.join(","), ...extra });
  return `/checkout?${q.toString()}`;
}
