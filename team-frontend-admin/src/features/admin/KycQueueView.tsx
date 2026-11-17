"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Link } from "@/components/ui/Link";
import { Table, type TableColumn } from "@/components/ui/Table";
import { useToast } from "@/components/ui/ToastProvider";
import type { KycSubmissionView } from "@/lib/gateway/kyc";
import { usePending } from "@/lib/usePending";

import { KycRejectDialog } from "./KycRejectDialog";
import { type KycStatus, formatVnSeconds } from "./casesParams";
import { reviewKycAction, viewKycDocumentAction } from "./kycActions";

const STATUS_LABELS: Record<KycStatus, string> = {
  PENDING: "Chờ duyệt",
  VERIFIED: "Đã xác minh",
  REJECTED: "Đã từ chối",
};

const FILTERS: { status: KycStatus; label: string }[] = [
  { status: "PENDING", label: "Đang chờ" },
  { status: "VERIFIED", label: "Đã được duyệt" },
  { status: "REJECTED", label: "Bị loại" },
];

function href(params: Record<string, string>) {
  return `/admin/cases?${new URLSearchParams(params).toString()}`;
}

function Documents({ item }: { item: KycSubmissionView }) {
  const [revealed, setRevealed] = useState<{
    url: string;
    expiresAt: string;
  } | null>(null);
  const [error, setError] = useState("");
  const { pending, run } = usePending();

  async function view(index: number) {
    if (pending) return;
    setError("");
    setRevealed(null);
    const fd = new FormData();
    fd.set("submissionId", item.id);
    fd.set("index", String(index));
    const res = await run(() => viewKycDocumentAction(fd));
    if (res.ok) setRevealed({ url: res.url, expiresAt: res.expiresAt });
    else setError(res.error);
  }

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap gap-2">
        {Array.from({ length: item.documentCount }, (_, i) => i + 1).map(
          (n) => (
            <Button
              key={n}
              size="sm"
              variant="outline"
              disabled={pending}
              onClick={() => view(n - 1)}
            >
              {`Xem tài liệu ${n}`}
            </Button>
          ),
        )}
      </div>
      {revealed ? (
        <p className="text-sm">
          <a
            href={revealed.url}
            target="_blank"
            rel="noopener noreferrer"
            className="underline"
          >
            Mở tài liệu
          </a>{" "}
          <span className="text-text-secondary">
            Hết hạn lúc {formatVnSeconds(revealed.expiresAt)}
          </span>
        </p>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function Actions({ item }: { item: KycSubmissionView }) {
  const [open, setOpen] = useState(false);
  const { pending, run } = usePending();
  const toast = useToast();

  async function approve() {
    if (pending) return;
    const fd = new FormData();
    fd.set("submissionId", item.id);
    fd.set("decision", "approve");
    const res = await run(() => reviewKycAction(fd));
    if (res.ok) toast.success(res.message);
    else toast.error(res.error);
  }

  return (
    <>
      <div className="flex gap-2">
        <Button size="sm" onClick={approve} disabled={pending}>
          Duyệt
        </Button>
        <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
          Từ chối
        </Button>
      </div>
      <KycRejectDialog
        submissionId={item.id}
        isOpen={open}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

const COLUMNS: TableColumn<KycSubmissionView>[] = [
  {
    key: "id",
    title: "Mã hồ sơ",
    render: (it) => <span className="font-mono text-sm">{it.id}</span>,
  },
  {
    key: "user",
    title: "Người bán",
    render: (it) => (
      <span className="whitespace-nowrap font-mono text-sm text-text-secondary">
        {it.userId}
      </span>
    ),
  },
  { key: "doc", title: "Loại giấy tờ", render: (it) => it.docType },
  {
    key: "created",
    title: "Gửi lúc",
    render: (it) => (
      <span className="whitespace-nowrap text-sm text-text-secondary">
        {formatVnSeconds(it.createdAt)}
      </span>
    ),
  },
  {
    key: "status",
    title: "Trạng thái",
    render: (it) => (
      <span className="text-sm">
        {STATUS_LABELS[it.status]}
        {it.reasonCode ? (
          <span className="block text-text-secondary">{it.reasonCode}</span>
        ) : null}
      </span>
    ),
  },
  {
    key: "docs",
    title: "Tài liệu",
    render: (it) => <Documents item={it} />,
  },
  {
    key: "actions",
    title: "Thao tác",
    render: (it) => (it.status === "PENDING" ? <Actions item={it} /> : null),
  },
];

export function KycQueueView({
  items,
  nextCursor,
  status,
}: {
  items: KycSubmissionView[];
  nextCursor: string;
  status: KycStatus;
}) {
  return (
    <div className="mt-6 space-y-4">
      <nav aria-label="Lọc hồ sơ xác minh" className="flex gap-3 text-sm">
        {FILTERS.map((f) => (
          <Link
            key={f.status}
            href={href({ tab: "kyc", status: f.status.toLowerCase() })}
            variant="secondary"
            size="sm"
            aria-current={f.status === status ? "true" : undefined}
          >
            {f.label}
          </Link>
        ))}
      </nav>
      <Card>
        {items.length === 0 ? (
          <p className="p-4 text-sm text-text-secondary">
            Không có hồ sơ nào đang chờ duyệt.
          </p>
        ) : (
          <Table<KycSubmissionView>
            caption="Hồ sơ xác minh người bán"
            columns={COLUMNS}
            dataSource={items}
            rowKey="id"
          />
        )}
      </Card>
      {nextCursor ? (
        <div className="flex justify-end">
          <Link
            href={href({
              tab: "kyc",
              status: status.toLowerCase(),
              cursor: nextCursor,
            })}
            variant="secondary"
            size="sm"
            fullWidth="mobile"
          >
            Trang sau
          </Link>
        </div>
      ) : null}
    </div>
  );
}
