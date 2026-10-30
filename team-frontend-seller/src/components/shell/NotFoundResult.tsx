import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";

export function NotFoundResult() {
  return (
    <Result
      status="404"
      title="Không tìm thấy trang"
      subTitle="Trang bạn đang tìm không tồn tại hoặc đã được di chuyển."
      extra={
        <Link href="/" variant="primary" size="sm">
          Về trang chủ
        </Link>
      }
    />
  );
}
