import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";
import { PlansView } from "@/features/admin/PlansView";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { listPlans } from "@/lib/gateway/campaigns";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Gói đăng ký người bán | Sàn Thương Mại Điện Tử",
  description: "Xem danh mục gói đăng ký của người bán.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

export default async function PlansPage() {
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/plans");
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Tài khoản của bạn không có quyền admin để xem gói đăng ký."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }
  const res = await listPlans();
  if (res.status === "unauthenticated") {
    redirectGuestToLogin("/admin/plans");
    return null;
  }
  if (res.status === "ok") return <PlansView plans={res.plans} />;
  return <PlansView plans={[]} error="unavailable" />;
}
