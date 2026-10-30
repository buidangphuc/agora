import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";

/** An unknown listing id (gateway NotFound): a product-specific 404 with a way out. */
export default function ListingNotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy sản phẩm"
      subTitle="Sản phẩm không tồn tại, đã bị gỡ hoặc đường dẫn không đúng."
      extra={
        <>
          <Link href="/" variant="primary" size="sm">
            Về trang chủ
          </Link>
          <Link href="/search" variant="outline" size="sm">
            Tìm sản phẩm khác
          </Link>
        </>
      }
    />
  );
}
