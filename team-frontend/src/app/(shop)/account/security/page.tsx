import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Badge } from "@/components/ui/Badge";
import { Card, CardHeader } from "@/components/ui/Card";
import { Pagination } from "@/components/ui/Pagination";
import { Table, type TableColumn } from "@/components/ui/Table";
import { Tag } from "@/components/ui/Tag";
import { AccountShell } from "@/features/account/AccountShell";
import { RetryButton } from "@/features/account/RetryButton";
import { RevokeSessionButton } from "@/features/account/RevokeSessionButton";
import { PAGE_SIZE, paginate, parsePage } from "@/features/account/pagination";
import { settle } from "@/features/account/settle";
import { getPrincipal } from "@/lib/gateway/session";
import {
  type ViewLoginEvent,
  type ViewSession,
  listLoginHistory,
  listSessions,
} from "@/lib/gateway/sessions";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Bảo mật tài khoản | Marketplace",
};

// Below `sm` the IP and time columns are hidden and repeated as a meta line
// under the first cell, so each row stacks instead of scrolling sideways.
const wide = "hidden sm:table-cell";
const meta = "mt-0.5 block text-xs font-normal text-text-secondary sm:hidden";

const sessionColumns: TableColumn<ViewSession>[] = [
  {
    key: "device",
    title: "Thiết bị",
    render: (s) => (
      <>
        <span className="font-medium">
          {s.device || "Thiết bị không xác định"}
        </span>
        <span className={meta}>
          IP: {s.ip || "—"} · Hoạt động: {s.lastSeen || s.createdAt}
        </span>
      </>
    ),
  },
  {
    key: "ip",
    title: "IP",
    className: wide,
    render: (s) => s.ip || "—",
  },
  {
    key: "lastSeen",
    title: "Hoạt động lần cuối",
    className: wide,
    render: (s) => s.lastSeen || s.createdAt,
  },
  {
    key: "action",
    title: "Thao tác",
    align: "right",
    render: (s) =>
      s.revoked ? (
        <Tag className="whitespace-nowrap">Đã thu hồi</Tag>
      ) : (
        <RevokeSessionButton sessionId={s.id} device={s.device} />
      ),
  },
];

const historyColumns: TableColumn<ViewLoginEvent>[] = [
  {
    key: "userAgent",
    title: "Trình duyệt",
    render: (e) => (
      <>
        <span className="break-words">{e.userAgent || "—"}</span>
        <span className={meta}>
          IP: {e.ip || "—"} · {e.createdAt}
        </span>
      </>
    ),
  },
  { key: "ip", title: "IP", className: wide, render: (e) => e.ip || "—" },
  {
    key: "createdAt",
    title: "Thời gian",
    className: wide,
    dataIndex: "createdAt",
  },
  {
    key: "result",
    title: "Kết quả",
    align: "right",
    render: (e) =>
      e.success ? (
        <Tag color="success" className="whitespace-nowrap">
          Thành công
        </Tag>
      ) : (
        <Tag color="danger" className="whitespace-nowrap">
          Thất bại
        </Tag>
      ),
  },
];

function SectionHeader({ title, count }: { title: string; count?: number }) {
  return (
    <CardHeader>
      <h2 className="text-base font-semibold text-text-primary">{title}</h2>
      {count !== undefined && <Badge variant="neutral">{count}</Badge>}
    </CardHeader>
  );
}

function ReadError({ what }: { what: string }) {
  return (
    <div className="p-5">
      <Alert
        type="error"
        description={`Không thể tải ${what}.`}
        action={<RetryButton />}
      />
    </div>
  );
}

export default async function SecurityPage({
  searchParams,
}: {
  searchParams?: { page?: string };
}) {
  if (!getPrincipal()) redirect("/login");

  const [sessions, history] = await Promise.all([
    settle(listSessions),
    settle(listLoginHistory),
  ]);
  const { rows: historyRows, page } = paginate(
    history.data ?? [],
    parsePage(searchParams?.page),
  );

  return (
    <AccountShell
      current="security"
      title="Bảo mật tài khoản"
      description="Quản lý các phiên đăng nhập và xem lịch sử đăng nhập của bạn."
    >
      <Card>
        <SectionHeader title="Phiên đăng nhập" count={sessions.data?.length} />
        {sessions.failed ? (
          <ReadError what="danh sách phiên đăng nhập" />
        ) : (
          <div className="p-5">
            <Table
              caption="Phiên đăng nhập"
              columns={sessionColumns}
              dataSource={sessions.data}
              rowKey="id"
              emptyText="Không có phiên nào đang hoạt động."
            />
          </div>
        )}
      </Card>

      <Card>
        <SectionHeader title="Lịch sử đăng nhập" count={history.data?.length} />
        {history.failed ? (
          <ReadError what="lịch sử đăng nhập" />
        ) : (
          <div className="space-y-4 p-5">
            <Table
              caption="Lịch sử đăng nhập"
              columns={historyColumns}
              dataSource={historyRows}
              rowKey="id"
              emptyText="Chưa có lịch sử đăng nhập."
            />
            <Pagination
              current={page}
              total={history.data.length}
              pageSize={PAGE_SIZE}
              hrefFor={(p) =>
                p === 1 ? "/account/security" : `/account/security?page=${p}`
              }
            />
          </div>
        )}
      </Card>
    </AccountShell>
  );
}
