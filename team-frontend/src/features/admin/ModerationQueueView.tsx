"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Link } from "@/components/ui/Link";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { usePending } from "@/features/seller/usePending";

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

const HEADERS = [
  "Tin đăng",
  "Người bán",
  "Giá",
  "Gửi lúc",
  "Cờ kiểm tra",
  "Thao tác",
];

function nextHref(basePath: string, tab: string | undefined, cursor: string) {
  const q = new URLSearchParams();
  if (tab) q.set("tab", tab);
  q.set("cursor", cursor);
  return `${basePath}?${q.toString()}`;
}

function Row({ item }: { item: QueueItem }) {
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
    <tr className="align-top">
      <td className="p-2">{item.title}</td>
      <td className="p-2">{item.sellerId}</td>
      <td className="p-2">{formatPrice(item.price)}</td>
      <td className="p-2">{formatVnSeconds(item.submittedAt)}</td>
      <td className="p-2">{item.screenFlags.join(", ")}</td>
      <td className="p-2">
        <div className="flex gap-2">
          <Button size="xs" onClick={approve} disabled={pending}>
            Duyệt
          </Button>
          <Button
            size="xs"
            variant="outline"
            onClick={() => setDialog("reject")}
          >
            Từ chối
          </Button>
          <Button size="xs" variant="danger" onClick={() => setDialog("ban")}>
            Khóa
          </Button>
        </div>
        <ModerateDialog
          listingId={item.id}
          decision={dialog ?? undefined}
          isOpen={dialog !== null}
          onClose={() => setDialog(null)}
        />
      </td>
    </tr>
  );
}

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
    <div className="mx-auto max-w-6xl p-6">
      {items.length === 0 ? (
        <p className="mt-4 text-sm text-text-secondary">Hàng chờ trống</p>
      ) : (
        <table className="mt-4 w-full text-left text-sm">
          <thead>
            <tr>
              {HEADERS.map((h) => (
                <th key={h} scope="col" className="p-2">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((it) => (
              <Row key={it.id} item={it} />
            ))}
          </tbody>
        </table>
      )}
      {nextCursor ? (
        <span className="mt-4 inline-block text-sm text-text-secondary">
          <Link href={nextHref(basePath, tab, nextCursor)} variant="subtle">
            Trang sau
          </Link>
        </span>
      ) : null}
    </div>
  );
}
