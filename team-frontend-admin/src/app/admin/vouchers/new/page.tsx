import type { Metadata } from "next";

import { Link } from "@/components/ui/Link";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { CreateVoucherForm } from "@/features/vouchers/CreateVoucherForm";
import {
  NO_ADMIN_SCOPE,
  VoucherForbidden,
} from "@/features/vouchers/VoucherForbidden";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Tạo voucher sàn | Sàn Thương Mại Điện Tử",
  description: "Tạo voucher áp dụng cho toàn sàn.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

export default async function NewVoucherPage() {
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/vouchers/new");
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return <VoucherForbidden subTitle={NO_ADMIN_SCOPE} />;
  }
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 md:gap-8">
      <header className="flex flex-col items-start gap-3">
        <div className="-ml-4">
          <Link href="/admin/vouchers" variant="ghost" size="sm">
            Về danh sách voucher sàn
          </Link>
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
          Tạo voucher sàn
        </h1>
        <p className="text-base text-text-secondary">
          Voucher áp dụng cho mọi người mua trên toàn sàn. Kiểm tra kỹ trước khi
          tạo vì chưa thể sửa hay ngừng voucher sau đó.
        </p>
      </header>
      <CreateVoucherForm />
    </div>
  );
}
