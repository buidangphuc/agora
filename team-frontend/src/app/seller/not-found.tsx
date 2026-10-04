import { Result } from "@/components/ui/Result";
import { LinkButton } from "@/features/seller/LinkButton";

export default function SellerNotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy nội dung"
      subTitle="Nội dung bạn tìm không tồn tại hoặc không thuộc gian hàng của bạn."
      extra={<LinkButton href="/seller">Về Kênh người bán</LinkButton>}
    />
  );
}
