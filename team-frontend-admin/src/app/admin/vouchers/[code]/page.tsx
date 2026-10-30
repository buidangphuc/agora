import type { Metadata } from "next";

import { LinkButton } from "@/components/LinkButton";
import { Alert } from "@/components/ui/Alert";
import { Card } from "@/components/ui/Card";
import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { VoucherDetails } from "@/features/vouchers/VoucherDetails";
import {
  GATEWAY_DENIED,
  NO_ADMIN_SCOPE,
  VoucherForbidden,
} from "@/features/vouchers/VoucherForbidden";
import { getPrincipal, getToken } from "@/lib/gateway/session";
import { getVoucher } from "@/lib/gateway/vouchers";

export const metadata: Metadata = {
  title: "Chi tiết voucher sàn | Sàn Thương Mại Điện Tử",
  description: "Thông tin một voucher áp dụng cho toàn sàn.",
  referrer: "no-referrer",
};

export const dynamic = "force-dynamic";

const MAX_CODE = 64;

function decodeCode(raw: string): string {
  try {
    return decodeURIComponent(raw);
  } catch {
    return "";
  }
}

function NotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy voucher"
      subTitle="Mã voucher không tồn tại hoặc không hợp lệ."
      extra={
        <LinkButton href="/admin/vouchers">Về danh sách voucher sàn</LinkButton>
      }
    />
  );
}

export default async function VoucherDetailPage({
  params,
}: {
  params: Promise<{ code: string }>;
}) {
  const { code: raw } = await params;
  const code = decodeCode(raw);
  const path = `/admin/vouchers/${encodeURIComponent(code)}`;
  const principal = getPrincipal();
  const token = getToken();
  if (!principal || !token) {
    redirectGuestToLogin(path);
    return null;
  }
  if (!principal.scopes.includes("admin")) {
    return <VoucherForbidden subTitle={NO_ADMIN_SCOPE} />;
  }
  if (!code || code.length > MAX_CODE) return <NotFound />;

  const result = await getVoucher(code);
  if (result.status === "unauthenticated") {
    redirectGuestToLogin(path);
    return null;
  }
  if (result.status === "forbidden") {
    return <VoucherForbidden subTitle={GATEWAY_DENIED} />;
  }
  if (result.status === "not_found" || result.status === "invalid") {
    return <NotFound />;
  }

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 md:gap-8">
      <header className="flex flex-col items-start gap-3">
        <div className="-ml-4">
          <Link href="/admin/vouchers" variant="ghost" size="sm">
            Về danh sách voucher sàn
          </Link>
        </div>
        <h1 className="break-all text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
          Voucher {result.status === "ok" ? result.voucher.code : code}
        </h1>
      </header>
      {result.status === "ok" ? (
        <Card padded>
          <VoucherDetails voucher={result.voucher} now={new Date()} />
        </Card>
      ) : (
        <Alert
          type="error"
          title="Không tải được voucher"
          description="Dịch vụ khuyến mãi đang tạm ngưng hoặc không phản hồi. Vui lòng tải lại trang sau."
        />
      )}
    </div>
  );
}
