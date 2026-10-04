"use client";

import { useRouter } from "next/navigation";
import {
  type ReactNode,
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from "react";

import { useToast } from "@/components/ui/ToastProvider";
import { type ResolvedVariant, resolveVariant } from "@/features/listing/pdp";
import { trackEcommerce } from "@/lib/analytics";
import type { ViewVariant } from "@/lib/gateway/listings";
import { addToCartAction } from "./actions";

/** The plain (client-safe) slice of a listing the purchase UI needs. */
export interface PurchaseListing {
  id: string;
  title: string;
  categoryId: string;
  price: number;
  stock: number;
  currency: string;
  variants: ViewVariant[];
  /** Active flash-sale price for the listing, null/undefined when none. */
  salePrice?: number | null;
}

export type PurchasePending = "add" | "buy" | null;

export interface PurchaseState {
  listing: PurchaseListing;
  /** The selected variant with its resolved price, stock, sku and image. */
  selected: ResolvedVariant;
  /** The price the buyer sees: the flash-sale price when there is one. */
  displayPrice: number;
  quantity: number;
  setQuantity: (quantity: number) => void;
  /** Select a variant locally; resets the quantity to 1. */
  selectVariant: (variantId: string) => void;
  /** Which request is running: both buttons are disabled while it is not null. */
  pending: PurchasePending;
  /** Add the selected variant; `buyNow` then goes to /checkout. */
  add: (buyNow: boolean) => Promise<void>;
}

const PurchaseContext = createContext<PurchaseState | null>(null);

/** Failure text from either the legacy `{ message }` or the shared `{ error }` result. */
function failureMessage(res: { error?: string; message?: string }): string {
  return res.error || res.message || "Thêm vào giỏ hàng thất bại.";
}

/**
 * Shares the selected variant, quantity and the add-to-cart pending state
 * between the inline PurchasePanel, the VariantSelector, the ImageGallery and
 * the mobile BuyBar, so there is exactly one source of truth for all of them.
 * `initialVariantId` is the server-resolved variant (from `?variant=`); a new
 * value (back/forward navigation) resets the selection.
 */
export function PurchaseProvider({
  listing,
  initialVariantId,
  children,
}: {
  listing: PurchaseListing;
  initialVariantId: string;
  children: ReactNode;
}) {
  const router = useRouter();
  const { success, error } = useToast();
  const [variantId, setVariantId] = useState(initialVariantId);
  const [seenInitial, setSeenInitial] = useState(initialVariantId);
  const [quantity, setQuantityState] = useState(1);
  const [pending, setPending] = useState<PurchasePending>(null);

  if (seenInitial !== initialVariantId) {
    setSeenInitial(initialVariantId);
    setVariantId(initialVariantId);
    setQuantityState(1);
  }

  const selected = useMemo(
    () => resolveVariant(listing, variantId),
    [listing, variantId],
  );

  const selectVariant = useCallback((id: string) => {
    setVariantId(id);
    setQuantityState(1);
  }, []);

  const setQuantity = useCallback((q: number) => setQuantityState(q), []);

  const add = useCallback(
    async (buyNow: boolean) => {
      if (pending !== null || selected.stock <= 0) return;
      setPending(buyNow ? "buy" : "add");
      try {
        const res = await addToCartAction(
          listing.id,
          selected.id || undefined,
          quantity,
        );
        if (res.ok) {
          trackEcommerce("add_to_cart", {
            currency: "VND",
            value: selected.price * quantity,
            items: [
              {
                itemId: listing.id,
                itemName: listing.title,
                price: selected.price,
                quantity,
                itemCategory: listing.categoryId,
              },
            ],
          });
          if (buyNow) router.push("/checkout");
          else success(`Đã thêm ${quantity} sản phẩm vào giỏ hàng`);
        } else {
          error(failureMessage(res));
        }
      } catch {
        error("Có lỗi xảy ra khi thêm vào giỏ hàng.");
      } finally {
        setPending(null);
      }
    },
    [pending, selected, listing, quantity, router, success, error],
  );

  const displayPrice = listing.salePrice ?? selected.price;

  const value = useMemo<PurchaseState>(
    () => ({
      listing,
      selected,
      displayPrice,
      quantity,
      setQuantity,
      selectVariant,
      pending,
      add,
    }),
    [
      listing,
      selected,
      displayPrice,
      quantity,
      setQuantity,
      selectVariant,
      pending,
      add,
    ],
  );

  return (
    <PurchaseContext.Provider value={value}>
      {children}
    </PurchaseContext.Provider>
  );
}

/** The purchase state; throws outside a PurchaseProvider. */
export function usePurchase(): PurchaseState {
  const ctx = useContext(PurchaseContext);
  if (!ctx) {
    throw new Error("usePurchase must be used inside a PurchaseProvider");
  }
  return ctx;
}

/** The purchase state, or null when rendered without a provider. */
export function useOptionalPurchase(): PurchaseState | null {
  return useContext(PurchaseContext);
}
