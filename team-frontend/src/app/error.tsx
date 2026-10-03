"use client";

import { Button } from "@/components/ui/Button";
import { Result } from "@/components/ui/Result";

export default function ErrorPage({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <Result
      status="error"
      title="Đã có lỗi xảy ra"
      subTitle="Không thể tải trang này. Vui lòng thử lại."
      extra={<Button onClick={() => reset()}>Thử lại</Button>}
    />
  );
}
