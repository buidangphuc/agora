const DEFAULT_BUYER_APP_URL = "http://localhost:3000";

export function buyerAppUrl(path = "/"): string {
  const base = (
    process.env.NEXT_PUBLIC_BUYER_APP_URL || DEFAULT_BUYER_APP_URL
  ).replace(/\/+$/, "");
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
}
