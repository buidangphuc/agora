import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { Result } from "@/components/ui/Result";
import { CockpitView } from "@/features/admin/CockpitView";
import { LinkButton } from "@/features/seller/LinkButton";
import { fetchCockpit } from "@/lib/gateway/cockpit";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Admin Operations Cockpit | Sàn Thương Mại Điện Tử",
  description:
    "Bản đồ vận hành thời gian thực, telemetry, Golden Signals và Jaeger Distributed Tracing.",
};

export const dynamic = "force-dynamic";

// Session check and first fetch both happen server-side. The scope decode is for
// gating the render only; the gateway enforces `admin` on the data itself.
export default async function CockpitPage() {
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirect("/login");
    return null;
  }

  if (!principal.scopes.includes("admin")) {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Tài khoản của bạn không có quyền admin để xem Operations Cockpit."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }

  const result = await fetchCockpit(token);
  if (result.status === "unauthenticated") {
    redirect("/login");
    return null;
  }
  if (result.status === "forbidden") {
    return (
      <Result
        status="403"
        title="Cần tài khoản Quản trị"
        subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này."
        extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
      />
    );
  }
  return <CockpitView initial={result.status === "ok" ? result.data : null} />;
}
