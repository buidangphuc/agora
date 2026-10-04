"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/ToastProvider";
import { clearCartAction } from "./actions";

export function ClearCartButton() {
  const [pending, setPending] = useState(false);
  const toast = useToast();

  async function handleClear() {
    if (!window.confirm("Bạn có chắc muốn xóa tất cả sản phẩm trong giỏ?")) {
      return;
    }
    setPending(true);
    try {
      const res = await clearCartAction();
      if (res.ok) toast.info("Đã làm trống giỏ hàng.");
      else toast.error(res.error);
    } catch {
      toast.error("Không thể xóa giỏ hàng. Vui lòng thử lại.");
    } finally {
      setPending(false);
    }
  }

  return (
    <Button variant="ghost" size="sm" isLoading={pending} onClick={handleClear}>
      Xóa tất cả
    </Button>
  );
}
