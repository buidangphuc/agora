import { Result } from "@/components/ui/Result";
import { LinkButton } from "@/features/seller/LinkButton";
import { buyerAppUrl } from "@/lib/apps";

export const metadata = {
  title: "Không có quyền | Kênh Người Bán",
};

export default function ForbiddenPage() {
  return (
    <Result
      status="403"
      title="Không có quyền"
      subTitle="Tài khoản của bạn chưa có quyền Người Bán (listing.write). Hãy kích hoạt kênh người bán để đăng bán sản phẩm."
      extra={
        <div className="space-y-3">
          <p className="font-semibold">Cần tài khoản Người Bán</p>
          <p>
            Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh người
            bán cho đến khi tài khoản được khôi phục.
          </p>
          <div className="flex flex-wrap justify-center gap-3">
            <LinkButton href="/sell">Đăng ký bán hàng</LinkButton>
            <a
              href={buyerAppUrl("/")}
              className="text-sm text-text-secondary hover:text-text-primary"
            >
              Về trang mua sắm
            </a>
          </div>
        </div>
      }
    />
  );
}
