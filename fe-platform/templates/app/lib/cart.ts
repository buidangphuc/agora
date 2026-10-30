import type { Cart } from "@fe-platform/api-client";
import { formatMinor } from "./money";

export type CartLineView = {
  id: string;
  listingId: string;
  title: string;
  variantName: string;
  quantity: number;
  unitPrice: string;
  imageUrl: string;
};

export type CartView = {
  lines: CartLineView[];
  subtotal: string;
  empty: boolean;
};

export function toCartView(cart: Cart | undefined): CartView {
  const items = cart?.items ?? [];
  return {
    lines: items.map((i) => ({
      id: i.id,
      listingId: i.listingId,
      title: i.title,
      variantName: i.variantName,
      quantity: i.quantity,
      unitPrice: formatMinor(i.unitPrice),
      imageUrl: i.imageUrl,
    })),
    subtotal: formatMinor(cart?.subtotal ?? 0n),
    empty: items.length === 0,
  };
}
