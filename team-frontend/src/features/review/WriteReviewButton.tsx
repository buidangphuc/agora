"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { ReviewModal } from "./ReviewModal";

/** "Viết đánh giá": opens the review modal. Client leaf. */
export function WriteReviewButton({
  listingId,
  productTitle,
  variant = "outline",
}: {
  listingId: string;
  productTitle?: string;
  variant?: "outline" | "primary";
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        data-testid="write-review"
        variant={variant}
        size="md"
        onClick={() => setOpen(true)}
      >
        Viết đánh giá
      </Button>
      {open && (
        <ReviewModal
          listingId={listingId}
          productTitle={productTitle}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
