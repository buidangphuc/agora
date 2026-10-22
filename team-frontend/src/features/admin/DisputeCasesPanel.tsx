"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/Button";
import { resolveDisputeAction } from "@/features/dispute/actions";
import {
  type DisputeAction,
  allowedDisputeActions,
  needsRuling,
} from "@/features/dispute/allowedActions";
import type { DisputeRulingName, ViewDispute } from "@/lib/gateway/disputes";

import { formatVnSeconds } from "./casesParams";

export type DisputeFilter = "open" | "investigating" | "resolved" | "rejected";

const FILTERS: [DisputeFilter, string][] = [
  ["open", "Chờ xử lý"],
  ["investigating", "Đang điều tra"],
  ["resolved", "Đã giải quyết"],
  ["rejected", "Đã từ chối"],
];

const CONFIRM =
  "Xác nhận phán quyết người mua thắng? Yêu cầu trả hàng sẽ được duyệt; nếu đơn đã thanh toán trực tuyến, người mua sẽ được hoàn tiền.";

const LABEL: Record<DisputeAction, string> = {
  investigate: "Bắt đầu điều tra",
  note: "Cập nhật ghi chú",
  resolve: "Giải quyết",
  reject: "Từ chối",
};

const TARGET = {
  investigate: "INVESTIGATING",
  note: "INVESTIGATING",
  resolve: "RESOLVED",
  reject: "REJECTED",
} as const;

function isSafeUrl(u: string): boolean {
  try {
    const p = new URL(u).protocol;
    return p === "http:" || p === "https:";
  } catch {
    return false;
  }
}

function DisputeCard({
  dispute,
  detail,
}: {
  dispute: ViewDispute;
  detail: boolean;
}) {
  const router = useRouter();
  const [note, setNote] = useState(dispute.resolution);
  const [ruling, setRuling] = useState<DisputeRulingName>("");
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const inFlight = useRef(false);
  const actions = allowedDisputeActions(dispute);
  const withRuling = needsRuling(dispute);
  const noteId = `note-${dispute.id}${detail ? "-detail" : ""}`;
  const rulingId = `ruling-${dispute.id}${detail ? "-detail" : ""}`;

  async function send(action: DisputeAction) {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError("");
    try {
      const sendRuling =
        action === "resolve" && withRuling && ruling !== ""
          ? ruling
          : undefined;
      const res = await resolveDisputeAction({
        disputeId: dispute.id,
        status: TARGET[action],
        resolution: note,
        ...(sendRuling ? { ruling: sendRuling } : {}),
      });
      if (res.ok) {
        router.push(`/admin/cases?tab=disputes&dispute=${dispute.id}`);
      } else {
        setError(res.error);
        setConfirming(false);
      }
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  function onAction(action: DisputeAction) {
    setError("");
    if (action === "resolve" && withRuling) {
      if (ruling === "") {
        setError("Vui lòng chọn phán quyết.");
        return;
      }
      if (ruling === "BUYER_FAVOUR") {
        setConfirming(true);
        return;
      }
    }
    void send(action);
  }

  return (
    <li className="rounded border p-3 text-sm">
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        <span>Đơn: {dispute.orderId}</span>
        <span>Người khiếu nại: {dispute.claimantId}</span>
        <span>Bên bị khiếu nại: {dispute.defendantId}</span>
        <span>{formatVnSeconds(dispute.createdAt)}</span>
        {detail ? (
          <span>{`Trạng thái: ${dispute.statusText}`}</span>
        ) : (
          <span>{dispute.statusText}</span>
        )}
        {dispute.rulingText ? <span>{dispute.rulingText}</span> : null}
      </div>
      <p className="mt-2">{dispute.reason}</p>
      {dispute.resolution ? (
        <p className="mt-1 text-text-secondary">{dispute.resolution}</p>
      ) : null}
      {dispute.evidenceUrls.length > 0 ? (
        <ul className="mt-2 space-y-1">
          {dispute.evidenceUrls.map((u) => (
            <li key={u}>
              {isSafeUrl(u) ? (
                <a href={u} target="_blank" rel="noopener noreferrer">
                  {u}
                </a>
              ) : (
                <span>{u}</span>
              )}
            </li>
          ))}
        </ul>
      ) : null}
      {actions.length > 0 ? (
        <div className="mt-3 space-y-2">
          <div>
            <label htmlFor={noteId} className="block text-xs">
              Ghi chú xử lý
            </label>
            <textarea
              id={noteId}
              className="w-full rounded border p-1"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          {withRuling && actions.includes("resolve") ? (
            <div>
              <label htmlFor={rulingId} className="block text-xs">
                Phán quyết
              </label>
              <select
                id={rulingId}
                className="rounded border p-1"
                value={ruling}
                onChange={(e) => {
                  setRuling(e.target.value as DisputeRulingName);
                  setConfirming(false);
                }}
              >
                <option value="">Chọn phán quyết</option>
                <option value="BUYER_FAVOUR">Người mua thắng</option>
                <option value="SELLER_FAVOUR">Người bán thắng</option>
              </select>
            </div>
          ) : null}
          {confirming ? (
            <div role="alertdialog" className="space-y-2">
              <p>{CONFIRM}</p>
              <div className="flex gap-2">
                <Button
                  size="xs"
                  disabled={pending}
                  onClick={() => void send("resolve")}
                >
                  Xác nhận
                </Button>
                <Button
                  size="xs"
                  variant="outline"
                  disabled={pending}
                  onClick={() => setConfirming(false)}
                >
                  Hủy
                </Button>
              </div>
            </div>
          ) : null}
          <div className="flex gap-2">
            {actions.map((a) => (
              <Button
                key={a}
                size="xs"
                variant={a === "reject" ? "outline" : undefined}
                disabled={pending}
                onClick={() => onAction(a)}
              >
                {LABEL[a]}
              </Button>
            ))}
          </div>
        </div>
      ) : null}
      {error ? (
        <p role="alert" className="mt-2 text-danger">
          {error}
        </p>
      ) : null}
    </li>
  );
}

export function DisputeCasesPanel({
  disputes,
  nextCursor,
  status,
  detail,
}: {
  disputes: ViewDispute[];
  nextCursor: string;
  status: DisputeFilter;
  detail?: ViewDispute;
}) {
  return (
    <div className="mx-auto max-w-6xl p-6">
      <h2 className="mb-3 text-lg font-semibold">Khiếu nại cần xử lý</h2>
      <div className="flex gap-3 text-sm">
        <span className="text-text-secondary">Trạng thái</span>
        {FILTERS.map(([s, label]) => (
          <Link
            key={s}
            href={`/admin/cases?tab=disputes&status=${s}`}
            aria-current={s === status ? "true" : undefined}
            className={s === status ? "font-semibold" : "text-text-secondary"}
          >
            {label}
          </Link>
        ))}
      </div>
      {detail ? (
        <ul className="mt-4">
          <DisputeCard
            key={`detail-${detail.id}-${detail.status}`}
            dispute={detail}
            detail
          />
        </ul>
      ) : null}
      {disputes.length === 0 ? (
        detail ? null : (
          <p className="mt-4 text-sm text-text-secondary">
            Không có khiếu nại nào
          </p>
        )
      ) : (
        <ul className="mt-4 space-y-3">
          {disputes.map((d) => (
            <DisputeCard
              key={`${d.id}-${d.status}`}
              dispute={d}
              detail={false}
            />
          ))}
        </ul>
      )}
      {nextCursor ? (
        <Link
          href={`/admin/cases?tab=disputes&status=${status}&cursor=${encodeURIComponent(nextCursor)}`}
          className="mt-4 inline-block text-sm text-text-secondary"
        >
          Trang sau
        </Link>
      ) : null}
    </div>
  );
}
