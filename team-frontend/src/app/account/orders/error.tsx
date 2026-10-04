"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";

/** Route error boundary for the order list: an Alert with a retry action. */
export default function OrdersError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <section className="mx-auto max-w-4xl py-2">
      <Alert
        type="error"
        title="Không tải được đơn hàng"
        description="Đã có lỗi xảy ra. Vui lòng thử lại."
        action={
          <Button variant="outline" size="sm" onClick={reset}>
            Thử lại
          </Button>
        }
      />
    </section>
  );
}
