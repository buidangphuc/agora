"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { useRouteRetry } from "@/lib/useRouteRetry";

/** Any non-NotFound failure while loading a product: a recoverable error with a retry. */
export default function ListingError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const retry = useRouteRetry(reset);
  return (
    <div className="mx-auto max-w-xl py-12">
      <Alert
        type="error"
        title="Không thể tải sản phẩm"
        description="Đã có lỗi khi tải trang sản phẩm. Vui lòng thử lại."
        action={
          <Button variant="outline" size="sm" onClick={retry}>
            Thử lại
          </Button>
        }
      />
    </div>
  );
}
