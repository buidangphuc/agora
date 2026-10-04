"use client";

import { type KeyboardEvent, type ReactNode, useRef, useState } from "react";

import { Image } from "@/components/ui/Image";
import { useOptionalPurchase } from "@/features/cart/PurchaseContext";
import { getImageUrl } from "@/lib/media";

const SWIPE_PX = 40;

/**
 * Product gallery: a fixed 1:1 `Image` stage (eager, above the fold) and a
 * strip of fixed 64px lazy thumbnails. Selection by click / Enter / Space and
 * Left/Right arrows, or a horizontal swipe on the stage. The selected variant's
 * image (from the PurchaseProvider, when there is one) wins until a thumbnail
 * is picked. A missing or failed image renders the `Image` fallback in the same
 * box, so the layout never shifts.
 */
export function ImageGallery({
  images,
  alt,
  overlay,
}: {
  /** Resolved image URLs (via getImageUrl); may be empty. */
  images: string[];
  alt: string;
  /** Bottom-right stage overlay, e.g. the favourite button. */
  overlay?: ReactNode;
}) {
  const purchase = useOptionalPurchase();
  const variantImage = purchase?.selected.imageUrl
    ? getImageUrl(purchase.selected.imageUrl)
    : "";

  const [picked, setPicked] = useState<number | null>(null);
  // Picking a thumbnail overrides the variant image until the variant changes.
  const [seenVariant, setSeenVariant] = useState(variantImage);
  if (seenVariant !== variantImage) {
    setSeenVariant(variantImage);
    setPicked(null);
  }
  const thumbs = useRef<(HTMLButtonElement | null)[]>([]);
  const touchStartX = useRef<number | null>(null);

  const stageSrc =
    picked !== null && images[picked] !== undefined
      ? images[picked]
      : variantImage || images[0] || "";
  const currentIndex = images.indexOf(stageSrc);

  function select(index: number, focus = false) {
    const next = (index + images.length) % images.length;
    setPicked(next);
    if (focus) thumbs.current[next]?.focus();
  }

  function onThumbKey(e: KeyboardEvent, index: number) {
    if (e.key === "ArrowRight") {
      e.preventDefault();
      select(index + 1, true);
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      select(index - 1, true);
    }
  }

  function onTouchEnd(clientX: number) {
    const start = touchStartX.current;
    touchStartX.current = null;
    if (start === null || images.length < 2) return;
    const delta = clientX - start;
    if (Math.abs(delta) < SWIPE_PX) return;
    select((currentIndex < 0 ? 0 : currentIndex) + (delta < 0 ? 1 : -1));
  }

  return (
    <div className="space-y-3">
      <div
        data-testid="gallery-stage"
        className="relative w-full overflow-hidden rounded-xl border border-border-subtle"
        onTouchStart={(e) => {
          touchStartX.current = e.touches[0]?.clientX ?? null;
        }}
        onTouchEnd={(e) => onTouchEnd(e.changedTouches[0]?.clientX ?? 0)}
      >
        <Image
          src={stageSrc}
          alt={alt}
          aspect="square"
          loading="eager"
          className="rounded-none"
        />
        {overlay && <div className="absolute bottom-2 right-2">{overlay}</div>}
      </div>

      {images.length > 1 && (
        <>
          <ul
            aria-label="Ảnh sản phẩm"
            className="flex gap-2 overflow-x-auto p-1"
          >
            {images.map((src, index) => {
              const current = index === currentIndex;
              return (
                <li key={src} className="shrink-0">
                  <button
                    type="button"
                    ref={(el) => {
                      thumbs.current[index] = el;
                    }}
                    aria-label={`Ảnh ${index + 1}`}
                    aria-current={current ? "true" : undefined}
                    onClick={() => select(index)}
                    onKeyDown={(e) => onThumbKey(e, index)}
                    className={`block h-16 w-16 overflow-hidden rounded-lg border-2 transition duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2 ${
                      current
                        ? "border-action-primary"
                        : "border-transparent hover:border-border-strong"
                    }`}
                  >
                    <Image
                      src={src}
                      alt=""
                      aspect="square"
                      className="rounded-none"
                    />
                  </button>
                </li>
              );
            })}
          </ul>
          {/* Mobile dot indicator: the swipe position, decorative. */}
          <div
            aria-hidden="true"
            className="flex justify-center gap-1.5 lg:hidden"
          >
            {images.map((src, index) => (
              <span
                key={src}
                className={`h-1.5 w-1.5 rounded-full ${
                  index === currentIndex
                    ? "bg-action-primary"
                    : "bg-border-strong"
                }`}
              />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
