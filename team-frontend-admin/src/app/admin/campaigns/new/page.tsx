import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { CreateCampaignForm } from "@/features/campaigns/CreateCampaignForm";
import { isPromotionAdmin } from "@/lib/gateway/scopes";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Tạo flash sale",
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
  if (!isPromotionAdmin(principal)) {
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
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 md:gap-8">
      <header className="flex flex-col items-start gap-3">
        <div className="-ml-4">
          <Link href="/admin/campaigns" variant="ghost" size="sm">
            Danh sách chiến dịch
          </Link>
        </div>
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
          Tạo flash sale
        </h1>
        <p className="text-base text-text-secondary">
          Đặt giá flash sale cho một tin đăng trong một khoảng thời gian.
        </p>
      </header>
      <CreateCampaignForm />
    </div>
  );
}
