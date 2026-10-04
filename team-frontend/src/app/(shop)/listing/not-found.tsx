import Link from "next/link";

import { Result } from "@/components/ui/Result";

const primary =
  "inline-flex items-center justify-center rounded-lg bg-action-primary px-4 py-2 text-sm font-medium text-text-inverse shadow-sm transition duration-150 hover:bg-action-primary-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2";
const secondary =
  "inline-flex items-center justify-center rounded-lg border border-border-strong bg-surface-card px-4 py-2 text-sm font-medium text-text-primary shadow-sm transition duration-150 hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2";

/** An unknown listing id (gateway NotFound): a product-specific 404 with a way out. */
export default function ListingNotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy sản phẩm"
      subTitle="Sản phẩm không tồn tại, đã bị gỡ hoặc đường dẫn không đúng."
      extra={
        <>
          <Link href="/" className={primary}>
            Về trang chủ
          </Link>
          <Link href="/search" className={secondary}>
            Tìm sản phẩm khác
          </Link>
        </>
      }
    />
  );
}
