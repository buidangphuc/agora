import Link from "next/link";

import { Result } from "@/components/ui/Result";

export function NotFoundResult() {
  return (
    <Result
      status="404"
      title="Không tìm thấy trang"
      subTitle="Trang bạn đang tìm không tồn tại hoặc đã được di chuyển."
      extra={
        <Link
          href="/"
          className="inline-flex items-center justify-center rounded-lg bg-action-primary px-4 py-2 text-sm font-medium text-text-inverse transition hover:bg-action-primary-hover"
        >
          Về trang chủ
        </Link>
      }
    />
  );
}
