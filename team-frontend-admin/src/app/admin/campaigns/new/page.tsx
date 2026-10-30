import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";
import { CreateCampaignForm } from "@/features/admin/CreateCampaignForm";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Tạo flash sale | Sàn Thương Mại Điện Tử",
  description: "Tạo chiến dịch flash sale cho một tin đăng.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

export default async function NewCampaignPage() {
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/campaigns/new");
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Tài khoản của bạn không có quyền admin để tạo chiến dịch."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }
  return (
    <div className="space-y-6">
      <Link href="/admin/campaigns" variant="subtle">
        Chiến dịch đang chạy
      </Link>
      <h1 className="text-2xl font-bold text-text-primary">Tạo flash sale</h1>
      <CreateCampaignForm />
    </div>
  );
}
