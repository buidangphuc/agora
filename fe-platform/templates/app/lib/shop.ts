export type ShopProduct = {
  id: string;
  title: string;
  description: string;
  priceMinor: number;
  currency: string;
};

export type CartLine = {
  product: ShopProduct;
  quantity: number;
  subtotalMinor: number;
};

export const SHOP_PRODUCTS: readonly ShopProduct[] = [
  {
    id: "desk-lamp",
    title: "Desk lamp",
    description: "Adjustable LED lamp with a warm and a cool setting.",
    priceMinor: 450000,
    currency: "VND",
  },
  {
    id: "notebook",
    title: "Notebook",
    description: "A5 dotted notebook with 160 pages.",
    priceMinor: 90000,
    currency: "VND",
  },
  {
    id: "mug",
    title: "Ceramic mug",
    description: "Hand glazed mug that holds 350 ml.",
    priceMinor: 120000,
    currency: "VND",
  },
];

export const SHOP_CURRENCY = "VND";

export function findShopProduct(id: string): ShopProduct | undefined {
  return SHOP_PRODUCTS.find((p) => p.id === id);
}

export function formatMoney(minor: number, currency: string = SHOP_CURRENCY) {
  return new Intl.NumberFormat("en", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(minor);
}

export function parseCart(raw: string | string[] | undefined): CartLine[] {
  const text = Array.isArray(raw) ? raw[0] : raw;
  if (!text) return [];
  const lines: CartLine[] = [];
  for (const part of text.split(",")) {
    const [id, qty] = part.split(":");
    const product = id ? findShopProduct(id) : undefined;
    const quantity = Number.parseInt(qty ?? "", 10);
    if (!product || !Number.isInteger(quantity) || quantity < 1) continue;
    const existing = lines.find((l) => l.product.id === product.id);
    if (existing) {
      existing.quantity += quantity;
      existing.subtotalMinor = existing.quantity * product.priceMinor;
    } else {
      lines.push({
        product,
        quantity,
        subtotalMinor: quantity * product.priceMinor,
      });
    }
  }
  return lines;
}

export function serializeCart(lines: readonly CartLine[]): string {
  return lines.map((l) => `${l.product.id}:${l.quantity}`).join(",");
}

export function addToCart(lines: readonly CartLine[], id: string): CartLine[] {
  const product = findShopProduct(id);
  if (!product) return [...lines];
  const next = lines.map((l) => ({ ...l }));
  const existing = next.find((l) => l.product.id === id);
  if (existing) {
    existing.quantity += 1;
    existing.subtotalMinor = existing.quantity * product.priceMinor;
  } else {
    next.push({ product, quantity: 1, subtotalMinor: product.priceMinor });
  }
  return next;
}

export function cartTotalMinor(lines: readonly CartLine[]): number {
  return lines.reduce((sum, l) => sum + l.subtotalMinor, 0);
}

export function cartQuery(lines: readonly CartLine[]): string {
  const raw = serializeCart(lines);
  return raw ? `?cart=${raw}` : "";
}
