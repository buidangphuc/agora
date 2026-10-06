"use client";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";

/** Any non-NotFound failure while loading a product: a recoverable error with a retry. */
export default function ListingError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div className="mx-auto max-w-xl py-12">
      <Alert
        type="error"
        title="Không thể tải sản phẩm"
        description="Đã có lỗi khi tải trang sản phẩm. Vui lòng thử lại."
        action={
          <Button variant="outline" size="sm" onClick={() => reset()}>
            Thử lại
          </Button>
        }
      />
    </div>
  );
}
