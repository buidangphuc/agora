"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useId } from "react";

import { Tag } from "@/components/ui/Tag";
import { usePurchase } from "@/features/cart/PurchaseContext";
import { shouldWriteVariantToUrl } from "./pdp";

/**
 * Variant picker: one native radio per variant, styled as a button chip (the
 * radio group gives arrow-key navigation and the checked state). Out-of-stock
 * variants are disabled with an "Hết hàng" tag. Selecting writes `?variant=<id>`
 * (`router.replace`, no scroll, no history entry) when the variant changes price
 * or stock relative to the base listing, so the server re-derives price, stock,
 * SKU and image; a variant identical to the base stays local. The quantity
 * resets to 1 on every change (PurchaseProvider.selectVariant).
 */
export function VariantSelector() {
  const { listing, selected, selectVariant } = usePurchase();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const name = useId();

  if (listing.variants.length === 0) return null;

  function onSelect(id: string) {
    if (id === selected.id) return;
    selectVariant(id);

    const params = new URLSearchParams(searchParams?.toString() ?? "");
    // Once the URL carries a variant it must keep tracking the selection,
    // otherwise a reload would restore an older choice.
    if (shouldWriteVariantToUrl(listing, id) || params.has("variant")) {
      params.set("variant", id);
    } else {
      return;
    }
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }

  return (
    <fieldset className="m-0 min-w-0 border-0 p-0">
      <legend className="mb-2 text-sm font-medium text-text-primary">
        Phân loại
      </legend>
      <div className="flex flex-wrap gap-2">
        {listing.variants.map((v) => {
          const out = v.stock <= 0;
          const checked = v.id === selected.id;
          return (
            <label
              key={v.id}
              className={`relative inline-flex ${
                out ? "cursor-not-allowed" : "cursor-pointer"
              }`}
            >
              <input
                type="radio"
                name={name}
                value={v.id}
                checked={checked}
                disabled={out}
                aria-disabled={out ? "true" : undefined}
                onChange={() => onSelect(v.id)}
                className="peer sr-only"
              />
              <span
                className={`inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm transition duration-150 peer-focus-visible:ring-2 peer-focus-visible:ring-focus-ring peer-focus-visible:ring-offset-2 ${
                  checked
                    ? "border-action-primary bg-primary-50 font-medium text-action-primary"
                    : "border-border-strong bg-surface-card text-text-primary"
                } ${out ? "opacity-50" : "hover:border-action-primary"}`}
              >
                <span>{v.name}</span>
                {out && <Tag color="neutral">Hết hàng</Tag>}
              </span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
