import React from "react";

import { Image } from "@/components/ui/Image";
import { getImageUrl } from "@/lib/media";

/** 1:1 item thumbnail in a fixed 64px box (80px from sm); the fallback placeholder keeps the box. */
export function OrderItemThumb({
  imageUrl,
  title,
  eager = false,
}: {
  imageUrl: string;
  title: string;
  /** Eager for images on the first screen, lazy below it. */
  eager?: boolean;
}) {
  const src = getImageUrl(imageUrl);
  return (
    <div className="w-16 shrink-0 sm:w-20">
      {src ? (
        <Image
          src={src}
          alt={title}
          aspect="square"
          loading={eager ? "eager" : "lazy"}
        />
      ) : (
        <div
          role="img"
          aria-label="Không có ảnh"
          className="aspect-square w-full rounded-2xl bg-surface-muted"
        />
      )}
    </div>
  );
}
