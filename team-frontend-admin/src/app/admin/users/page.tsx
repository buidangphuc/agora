import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";
import { UsersView } from "@/features/admin/UsersView";
import { parseUserFilters } from "@/features/admin/userFilters";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import { listUsers } from "@/lib/gateway/users";

export const metadata: Metadata = {
  title: "Quản lý người dùng | Sàn Thương Mại Điện Tử",
  description: "Tìm người dùng và hạn chế, tạm khóa hoặc cấm tài khoản.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

type SearchParams = Record<string, string | string[] | undefined>;

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

// Same gate order as the audit page. The scope decode only gates rendering;
// the gateway enforces `admin` on the data itself.
export default async function UsersPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const sp = await searchParams;
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin("/admin/users", sp);
    return null;
  }

  if (!principal.scopes.includes("admin")) {
    return (
      <Forbidden subTitle="Tài khoản của bạn không có quyền admin để quản lý người dùng." />
    );
  }

  const filters = parseUserFilters(sp);
  const result = await listUsers(filters, filters.cursor);
  if (result.status === "unauthenticated") {
    redirectGuestToLogin("/admin/users", sp);
    return null;
  }
  if (result.status === "forbidden") {
    return (
      <Forbidden subTitle="Gateway từ chối quyền admin cho phiên đăng nhập này." />
    );
  }
  if (result.status === "ok") {
    return (
      <UsersView
        users={result.users}
        nextCursor={result.nextCursor}
        filters={filters}
        selfId={principal.id}
      />
    );
  }
  return (
    <UsersView
      users={[]}
      nextCursor=""
      filters={filters}
      selfId={principal.id}
      error="unavailable"
    />
  );
}
