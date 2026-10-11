"use client";

import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";
import { useRouteRetry } from "@/lib/useRouteRetry";

/**
 * Route-level failure for the account screens: an error Result whose retry
 * re-renders the route segment via Next's `reset()`.
 */
export function RouteError({ reset }: { reset: () => void }) {
  const retry = useRouteRetry(reset);
  return (
    <Result
      status="error"
      title="Đã có lỗi xảy ra"
      subTitle="Không thể tải trang này. Vui lòng thử lại."
      extra={<Button onClick={retry}>Thử lại</Button>}
    />
  );
}
