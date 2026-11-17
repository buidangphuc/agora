"use client";

import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Table, type TableColumn } from "@/components/ui/Table";
import { Tag, type TagColor } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { AdCampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { ViewAdCampaign } from "@/lib/gateway/promotion";
import { pauseAdCampaignAction, resumeAdCampaignAction } from "./actions";

export type AdCampaignRow = ViewAdCampaign & { listingTitle?: string };

const COLORS: Partial<Record<AdCampaignStatus, TagColor>> = {
  [AdCampaignStatus.ACTIVE]: "success",
  [AdCampaignStatus.PAUSED]: "warning",
  [AdCampaignStatus.PAUSED_BUDGET]: "danger",
  [AdCampaignStatus.ENDED]: "neutral",
};

export function AdCampaignTable({
  campaigns,
}: {
  campaigns: AdCampaignRow[];
}) {
  const router = useRouter();
  const toast = useToast();
  const [overrides, setOverrides] = useState<Record<string, ViewAdCampaign>>(
    {},
  );
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const inFlight = useRef(new Set<string>());

  async function act(
    id: string,
    action: typeof pauseAdCampaignAction,
    success: string,
  ) {
    if (inFlight.current.has(id)) return;
    inFlight.current.add(id);
    setBusy((b) => ({ ...b, [id]: true }));
    try {
      const res = await action(id);
      if (res.ok) {
        const next = res.data?.campaign;
        if (next) setOverrides((o) => ({ ...o, [id]: next }));
        toast.success(success);
        router.refresh();
      } else {
        toast.error(res.error);
      }
    } finally {
      inFlight.current.delete(id);
      setBusy((b) => ({ ...b, [id]: false }));
    }
  }

  const rows = campaigns.map((c) => {
    const o = overrides[c.id];
    return o ? { ...c, status: o.status, statusText: o.statusText } : c;
  });

  const columns: TableColumn<AdCampaignRow>[] = [
    {
      key: "listing",
      title: "Sản phẩm",
      render: (c) => c.listingTitle || c.listingId,
    },
    {
      key: "budget",
      title: "Ngân sách còn lại",
      align: "right",
      render: (c) => formatPrice(Math.max(0, c.budget - c.spent)),
    },
    {
      key: "bid",
      title: "Giá thầu",
      align: "right",
      render: (c) => formatPrice(c.bid),
    },
    {
      key: "spent",
      title: "Đã chi",
      align: "right",
      render: (c) => formatPrice(c.spent),
    },
    {
      key: "impressions",
      title: "Lượt hiển thị",
      align: "right",
      render: (c) => c.impressions,
    },
    {
      key: "clicks",
      title: "Lượt nhấp",
      align: "right",
      render: (c) => c.clicks,
    },
    {
      key: "status",
      title: "Trạng thái",
      render: (c) => (
        <Tag color={COLORS[c.status] ?? "neutral"}>{c.statusText}</Tag>
      ),
    },
    {
      key: "actions",
      title: "Thao tác",
      align: "right",
      render: (c) => {
        if (c.status === AdCampaignStatus.ACTIVE) {
          return (
            <Button
              variant="outline"
              size="sm"
              disabled={busy[c.id]}
              onClick={() =>
                act(c.id, pauseAdCampaignAction, "Đã tạm dừng chiến dịch")
              }
            >
              Tạm dừng
            </Button>
          );
        }
        if (
          c.status === AdCampaignStatus.PAUSED ||
          c.status === AdCampaignStatus.PAUSED_BUDGET
        ) {
          return (
            <Button
              variant="outline"
              size="sm"
              disabled={
                busy[c.id] || c.status === AdCampaignStatus.PAUSED_BUDGET
              }
              onClick={() =>
                act(c.id, resumeAdCampaignAction, "Đã tiếp tục chiến dịch")
              }
            >
              Tiếp tục
            </Button>
          );
        }
        return null;
      },
    },
  ];

  return (
    <Table
      caption="Chiến dịch quảng cáo"
      columns={columns}
      dataSource={rows}
      rowKey="id"
      emptyText="Chưa có chiến dịch nào"
    />
  );
}
