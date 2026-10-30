"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Link } from "@/components/ui/Link";
import { Table, type TableColumn } from "@/components/ui/Table";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { usePending } from "@/lib/usePending";

import { ModerateDialog } from "./ModerateDialog";
import { formatVnSeconds } from "./casesParams";
import { moderateListingAction } from "./moderationActions";

export interface QueueItem {
  id: string;
  title: string;
  sellerId: string;
  price: number;
  submittedAt: string;
  screenFlags: string[];
}

function nextHref(basePath: string, tab: string | undefined, cursor: string) {
  const q = new URLSearchParams();
  if (tab) q.set("tab", tab);
  q.set("cursor", cursor);
  return `${basePath}?${q.toString()}`;
}

function QueueActions({ item }: { item: QueueItem }) {
  const [dialog, setDialog] = useState<"reject" | "ban" | null>(null);
  const { pending, run } = usePending();
  const toast = useToast();

  async function approve() {
    if (pending) return;
    const res = await run(() =>
      moderateListingAction(item.id, "approve", "", ""),
    );
    if (res.ok) toast.success(res.message);
    else toast.error(res.error);
  }

  return (
    <>
      <div className="flex gap-2">
        <Button size="sm" onClick={approve} disabled={pending}>
          Duyệt
        </Button>
        <Button size="sm" variant="outline" onClick={() => setDialog("reject")}>
          Từ chối
        </Button>
        <Button size="sm" variant="danger" onClick={() => setDialog("ban")}>
          Khóa
        </Button>
      </div>
      <ModerateDialog
        listingId={item.id}
        decision={dialog ?? undefined}
        isOpen={dialog !== null}
        onClose={() => setDialog(null)}
      />
    </>
  );
}

const COLUMNS: TableColumn<QueueItem>[] = [
  {
    key: "title",
    title: "Tin đăng",
    render: (it) => (
      <span className="block min-w-40 max-w-xs break-words font-semibold">
        {it.title}
      </span>
    ),
  },
  {
    key: "seller",
    title: "Người bán",
    render: (it) => (
      <span className="whitespace-nowrap font-mono text-sm text-text-secondary">
        {it.sellerId}
      </span>
    ),
  },
  {
    key: "price",
    title: "Giá",
    align: "right",
    render: (it) => (
      <span className="whitespace-nowrap font-semibold">
        {formatPrice(it.price)}
      </span>
    ),
  },
  {
    key: "submitted",
    title: "Gửi lúc",
    render: (it) => (
      <span className="whitespace-nowrap text-sm text-text-secondary">
        {formatVnSeconds(it.submittedAt)}
      </span>
    ),
  },
  {
    key: "flags",
    title: "Cờ kiểm tra",
    render: (it) => (
      <span className="block min-w-32 break-words text-sm">
        {it.screenFlags.join(", ")}
      </span>
    ),
  },
  {
    key: "actions",
    title: "Thao tác",
    render: (it) => <QueueActions item={it} />,
  },
];

export function ModerationQueueView({
  items,
  nextCursor,
  basePath,
  tab,
}: {
  items: QueueItem[];
  nextCursor: string;
  basePath: string;
  tab?: string;
}) {
  return (
    <div className="mt-6 space-y-4">
      <Card>
        {items.length === 0 ? (
          <Empty description="Hàng chờ trống" />
        ) : (
          <Table<QueueItem>
            caption="Hàng chờ kiểm duyệt"
            columns={COLUMNS}
            dataSource={items}
            rowKey="id"
          />
        )}
      </Card>
      {nextCursor ? (
        <div className="flex justify-end">
          <Link
            href={nextHref(basePath, tab, nextCursor)}
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
