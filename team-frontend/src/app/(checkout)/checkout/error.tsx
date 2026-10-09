"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { useRouteRetry } from "@/lib/useRouteRetry";

export default function CheckoutError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const retry = useRouteRetry(reset);
  return (
    <section className="mx-auto max-w-xl py-8">
      <Alert
        type="error"
        title="Không thể tải trang thanh toán"
        description="Đã có lỗi xảy ra. Giỏ hàng của bạn không bị thay đổi."
        action={
          <Button size="sm" variant="outline" onClick={retry}>
            Thử lại
          </Button>
        }
      />
    </section>
  );
}
