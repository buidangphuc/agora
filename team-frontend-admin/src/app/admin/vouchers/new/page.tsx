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
    <div className="flex flex-col gap-4">
      <Link href="/admin/vouchers" variant="subtle">
        Về danh sách voucher sàn
      </Link>
      <h1 className="text-2xl font-bold text-text-primary">Tạo voucher sàn</h1>
      <CreateVoucherForm />
    </div>
  );
}
