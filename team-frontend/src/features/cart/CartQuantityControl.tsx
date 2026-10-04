"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { QuantityPicker } from "@/components/ui/QuantityPicker";
import { useToast } from "@/components/ui/ToastProvider";
import { removeFromCartAction, updateCartItemAction } from "./actions";

/**
 * Quantity stepper + remove for one cart row. The quantity shown is always the
 * server value (`quantity` prop): the action revalidates /cart and the RSC tree
 * re-renders; nothing is mirrored in client state, so a failed update simply
 * leaves the previous value on screen.
 */
export function CartQuantityControl({
  itemId,
  quantity,
  title,
}: {
  itemId: string;
  quantity: number;
  title: string;
}) {
  const [pending, setPending] = useState<"update" | "remove" | null>(null);
  const toast = useToast();

  async function run(
    kind: "update" | "remove",
    call: () => ReturnType<typeof updateCartItemAction>,
    successMessage?: string,
  ) {
    setPending(kind);
    try {
      const res = await call();
      if (res.ok) {
        if (successMessage) toast.info(successMessage);
      } else {
        toast.error(res.error);
      }
    } catch {
      toast.error("Không thể cập nhật giỏ hàng. Vui lòng thử lại.");
    } finally {
      setPending(null);
    }
  }

  return (
    <div
      className="flex items-center gap-3"
      aria-busy={pending ? "true" : undefined}
    >
      <QuantityPicker
        size="sm"
        value={quantity}
        min={1}
        disabled={pending !== null}
        label={`Số lượng: ${title}`}
        onChange={(next) =>
          run("update", () => updateCartItemAction(itemId, next))
        }
      />
      <Button
        size="sm"
        variant="ghost"
        isLoading={pending === "remove"}
        disabled={pending !== null}
        aria-label={`Xóa ${title}`}
        onClick={() =>
          run(
            "remove",
            () => removeFromCartAction(itemId),
            "Đã xóa sản phẩm khỏi giỏ hàng.",
          )
        }
      >
        Xóa
      </Button>
    </div>
  );
}
