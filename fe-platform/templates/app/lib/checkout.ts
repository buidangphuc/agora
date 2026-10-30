import type { Address, PreviewCheckoutResponse } from "@fe-platform/api-client";
import { formatMinor } from "./money";

export type AddressView = {
  id: string;
  recipient: string;
  phone: string;
  line: string;
  isDefault: boolean;
};

export type QuoteGroupView = {
  sellerId: string;
  itemsSubtotal: string;
  shippingFee: string;
  freeShipping: boolean;
  total: string;
};

export type QuoteView = {
  groups: QuoteGroupView[];
  itemsSubtotal: string;
  shippingFee: string;
  shippingQuoted: boolean;
  discount: string;
  total: string;
};

export function toAddressView(a: Address): AddressView {
  return {
    id: a.id,
    recipient: a.recipientName,
    phone: a.phone,
    line: [a.street, a.ward, a.district, a.city].filter(Boolean).join(", "),
    isDefault: a.isDefault,
  };
}

export function toQuoteView(q: PreviewCheckoutResponse): QuoteView {
  return {
    groups: q.groups.map((g) => ({
      sellerId: g.sellerId,
      itemsSubtotal: formatMinor(g.itemsSubtotal),
      shippingFee: formatMinor(g.shippingFee),
      freeShipping: g.isFreeShipping,
      total: formatMinor(g.totalAmount),
    })),
    itemsSubtotal: formatMinor(q.itemsSubtotal),
    shippingFee: formatMinor(q.shippingFee),
    shippingQuoted: q.shippingQuoted,
    discount: formatMinor(q.discountAmount),
    total: formatMinor(q.totalAmount),
  };
}
