const DEFAULT_SELLER_APP_URL = "http://localhost:3001";

export function sellerAppUrl(path = "/seller"): string {
  const base = (
    process.env.NEXT_PUBLIC_SELLER_APP_URL || DEFAULT_SELLER_APP_URL
  ).replace(/\/+$/, "");
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
}

const SELLER_HOME = "/seller";

function sellerNext(next: string): string {
  if (!next.startsWith("/") || next.startsWith("//") || next.includes("\\"))
    return SELLER_HOME;
  return next;
}

export function sellerEntryUrl(next = SELLER_HOME): string {
  return sellerAppUrl(`/login?next=${encodeURIComponent(sellerNext(next))}`);
}
