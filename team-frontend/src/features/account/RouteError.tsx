"use client";

import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";

/**
 * Route-level failure for the account screens: an error Result whose retry
 * re-renders the route segment via Next's `reset()`.
 */
export function RouteError({ reset }: { reset: () => void }) {
  return (
    <Result
      status="error"
      title="Đã có lỗi xảy ra"
      subTitle="Không thể tải trang này. Vui lòng thử lại."
      extra={<Button onClick={() => reset()}>Thử lại</Button>}
    />
  );
}
