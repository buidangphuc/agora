import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Pagination } from "@/components/ui/Pagination";
import { AccountShell } from "@/features/account/AccountShell";
import { RetryButton } from "@/features/account/RetryButton";
import { PAGE_SIZE, paginate, parsePage } from "@/features/account/pagination";
import { settle } from "@/features/account/settle";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { ConsentForm } from "@/features/privacy/ConsentForm";
import { ConsentHistory } from "@/features/privacy/ConsentHistory";
import { DeletionCard } from "@/features/privacy/DeletionCard";
import { ExportButton } from "@/features/privacy/ExportButton";
import { getDeletionStatus, listConsents } from "@/lib/gateway/privacy";
import { getPrincipal } from "@/lib/gateway/session";
import Link from "next/link";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Quyền riêng tư",
};

function SectionTitle({ children }: { children: string }) {
  return (
    <CardHeader>
      <h2 className="text-base font-semibold text-text-primary">{children}</h2>
    </CardHeader>
  );
}

function ReadError({ what }: { what: string }) {
  return (
    <Alert
      type="error"
      description={`Không thể tải ${what}.`}
      action={<RetryButton />}
    />
  );
}

export default async function PrivacyPage(props: {
  searchParams?: Promise<{ page?: string }>;
}) {
  const searchParams = await props.searchParams;
  if (!getPrincipal()) redirectGuestToLogin("/account/privacy", searchParams);

  const [consents, deletion] = await Promise.all([
    settle(() => listConsents()),
    settle(getDeletionStatus),
  ]);
  const { rows, page } = paginate(
    consents.data?.records ?? [],
    parsePage(searchParams?.page),
  );

  return (
    <AccountShell
      current="privacy"
      title="Quyền riêng tư"
      description="Quản lý sự đồng ý, dữ liệu và tài khoản của bạn."
    >
      <Card>
        <SectionTitle>Lịch sử đồng ý</SectionTitle>
        <CardContent>
          {consents.failed ? (
            <ReadError what="lịch sử đồng ý" />
          ) : (
            <div className="space-y-4">
              <ConsentForm records={consents.data.records} />
              <ConsentHistory records={rows} />
              <Pagination
                current={page}
                total={consents.data.records.length}
                pageSize={PAGE_SIZE}
                hrefFor={(p) =>
                  p === 1
                    ? "/account/privacy"
                    : `/account/privacy?page=${p}`
                }
              />
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <SectionTitle>Xuất dữ liệu</SectionTitle>
        <CardContent>
          <div className="space-y-3">
            <p className="text-sm text-text-secondary">
              Tải về dữ liệu tài khoản của bạn. Dữ liệu{" "}
              <Link href="/account/orders" className="underline">
                đơn hàng
              </Link>{" "}
              và{" "}
              <Link href="/chat" className="underline">
                tin nhắn
              </Link>{" "}
              xem trực tiếp tại trang riêng.
            </p>
            <ExportButton />
          </div>
        </CardContent>
      </Card>

      <Card>
        <SectionTitle>Xóa tài khoản</SectionTitle>
        <CardContent>
          {deletion.failed ? (
            <ReadError what="trạng thái xóa tài khoản" />
          ) : (
            <DeletionCard status={deletion.data} />
          )}
        </CardContent>
      </Card>
    </AccountShell>
  );
}
