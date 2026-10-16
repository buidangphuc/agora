import type { Metadata } from "next";

import { Result } from "@/components/ui/Result";
import { ModerationQueueView } from "@/features/admin/ModerationQueueView";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { LinkButton } from "@/features/seller/LinkButton";
import { listModerationQueue } from "@/lib/gateway/moderation";
import { getPrincipal, getToken } from "@/lib/gateway/session";

export const metadata: Metadata = {
  title: "Hàng chờ kiểm duyệt | Sàn Thương Mại Điện Tử",
  description: "Duyệt, từ chối hoặc khóa các tin đăng đang chờ kiểm duyệt.",
};

export const dynamic = "force-dynamic";

function Forbidden({ subTitle }: { subTitle: string }) {
  return (
    <Result
      status="403"
      title="Cần tài khoản Quản trị"
      subTitle={subTitle}
      extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
    />
  );
}

// The scope decode only gates rendering; the gateway enforces `admin` itself.
export default async function ListingsPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const sp = await searchParams;
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/listings");
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return (
      <Forbidden subTitle="Tài khoản của bạn không có quyền admin để kiểm duyệt tin đăng." />
    );
  }

  const raw = Array.isArray(sp.cursor) ? sp.cursor[0] : sp.cursor;
  const cursor = (raw ?? "").trim().slice(0, 256);

  const result = await listModerationQueue(cursor);
  if (result.status === "unauthenticated") {
    redirectGuestToLogin("/admin/listings");
    return null;
  }
  if (result.status === "forbidden") {
    return (
      <Forbidden subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này." />
    );
  }
  if (result.status !== "ok") {
    return (
      <div className="mx-auto max-w-6xl p-6">
        <p role="alert" className="text-sm text-text-secondary">
          Không tải được hàng chờ kiểm duyệt. Vui lòng thử lại sau.
        </p>
      </div>
    );
  }
  return (
    <ModerationQueueView
      items={result.items}
      nextCursor={result.nextCursor}
      basePath="/admin/listings"
    />
  );
}
