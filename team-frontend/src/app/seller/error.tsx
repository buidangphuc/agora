"use client";

import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";
import { LinkButton } from "@/features/seller/LinkButton";

/** Seller segment error boundary: retry the render, or go back to the cockpit. */
export default function SellerError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <Result
      status="error"
      title="Không tải được trang này"
      subTitle="Đã có lỗi khi tải dữ liệu kênh người bán. Vui lòng thử lại."
      extra={
        <>
          <Button onClick={() => reset()}>Thử lại</Button>
          <LinkButton href="/seller">Về Kênh người bán</LinkButton>
        </>
      }
    />
  );
}
