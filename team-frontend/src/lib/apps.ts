const DEFAULT_SELLER_APP_URL = "http://localhost:3001";

export function sellerAppUrl(path = "/seller"): string {
  const base = (
    process.env.NEXT_PUBLIC_SELLER_APP_URL || DEFAULT_SELLER_APP_URL
  ).replace(/\/+$/, "");
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
}
