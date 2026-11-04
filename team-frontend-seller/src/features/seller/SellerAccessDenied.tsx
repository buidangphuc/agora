import { Result } from "@/components/ui/Result";
import { buyerAppUrl } from "@/lib/apps";

import { SellerLogoutButton } from "./SellerLogoutButton";

export function SellerAccessDenied(_props: { restricted?: boolean }) {
  return (
    <main className="mx-auto w-full max-w-3xl px-4 py-10">
      <Result
        status="403"
        title="Tài khoản Người Bán đang bị hạn chế"
        extra={
          <div className="space-y-3">
            <p>
              Tài khoản bị hạn chế? Người bán bị hạn chế không thể dùng kênh
              người bán cho đến khi tài khoản được khôi phục.
            </p>
            <div className="flex flex-wrap justify-center gap-3">
              <a
                href={buyerAppUrl("/")}
                className="text-sm text-text-secondary hover:text-text-primary"
              >
                Về trang mua sắm
              </a>
            </div>
            <SellerLogoutButton />
          </div>
        }
      />
    </main>
  );
}
