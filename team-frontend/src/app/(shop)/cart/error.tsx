"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { useRouteRetry } from "@/lib/useRouteRetry";

export default function CartError({
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
        title="Không thể tải giỏ hàng"
        description="Đã có lỗi xảy ra khi tải giỏ hàng. Vui lòng thử lại."
        action={
          <Button size="sm" variant="outline" onClick={retry}>
            Thử lại
          </Button>
        }
      />
    </section>
  );
}
