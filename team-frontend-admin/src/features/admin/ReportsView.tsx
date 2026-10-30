"use client";

import NextLink from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Link } from "@/components/ui/Link";

import { CloseReportDialog } from "./CloseReportDialog";
import { ModerateDialog } from "./ModerateDialog";
import type { CasesStatus } from "./casesParams";
import { formatVnSeconds } from "./casesParams";

export interface ReportItem {
  id: string;
  listingId: string;
  reasonCode: string;
  note: string;
  reporterId: string;
  status: "open" | "actioned" | "dismissed";
  createdAt: string;
  resolutionNote: string;
}

const REASON_LABEL: Record<string, string> = {
  prohibited_item: "Hàng cấm",
  counterfeit: "Hàng giả",
  scam: "Lừa đảo",
  misleading: "Thông tin gây hiểu lầm",
  inappropriate: "Nội dung không phù hợp",
  other: "Khác",
};
const STATUS_LABEL: Record<ReportItem["status"], string> = {
  open: "Đang mở",
  actioned: "Đã xử lý",
  dismissed: "Đã bác bỏ",
};
const FILTERS: [CasesStatus, string][] = [
  ["OPEN", "Đang mở"],
  ["ACTIONED", "Đã xử lý"],
  ["DISMISSED", "Đã bác bỏ"],
];
const HEADERS = [
  "Tin đăng",
  "Lý do",
  "Ghi chú",
  "Người báo cáo",
  "Báo cáo lúc",
  "Trạng thái",
  "Thao tác",
];

function Row({ report }: { report: ReportItem }) {
  const [moderate, setModerate] = useState(false);
  const [close, setClose] = useState(false);
  const [removed, setRemoved] = useState(false);
  return (
    <tr className="align-top">
      <td className="p-2">{report.listingId}</td>
      <td className="p-2">
        {REASON_LABEL[report.reasonCode] ?? report.reasonCode}
      </td>
      <td className="p-2">{report.note}</td>
      <td className="p-2">{report.reporterId}</td>
      <td className="p-2">{formatVnSeconds(report.createdAt)}</td>
      <td className="p-2">{STATUS_LABEL[report.status]}</td>
      <td className="p-2">
        <div className="flex items-center gap-2">
          {removed ? (
            <span className="text-sm text-text-secondary">
              Tin đăng đã bị gỡ
            </span>
          ) : (
            <Button size="xs" onClick={() => setModerate(true)}>
              Xử lý tin đăng
            </Button>
          )}
          <Button size="xs" variant="outline" onClick={() => setClose(true)}>
            Đóng báo cáo
          </Button>
        </div>
        <ModerateDialog
          listingId={report.listingId}
          isOpen={moderate}
          onClose={() => setModerate(false)}
          onResult={(res) => {
            if (!res.ok && res.error === "Tin đăng không còn tồn tại.") {
              setRemoved(true);
              setModerate(false);
            }
          }}
        />
        <CloseReportDialog
          reportId={report.id}
          isOpen={close}
          onClose={() => setClose(false)}
        />
      </td>
    </tr>
  );
}

export function ReportsView({
  reports,
  nextCursor,
  status,
}: {
  reports: ReportItem[];
  nextCursor: string;
  status: CasesStatus;
}) {
  return (
    <div className="mx-auto max-w-6xl p-6">
      <div className="flex gap-3 text-sm">
        {FILTERS.map(([s, label]) => (
          <NextLink
            key={s}
            href={`/admin/cases?tab=reports&status=${s}`}
            aria-current={s === status ? "true" : undefined}
            className={s === status ? "font-semibold" : "text-text-secondary"}
          >
            {label}
          </NextLink>
        ))}
      </div>
      {reports.length === 0 ? (
        <p className="mt-4 text-sm text-text-secondary">Không có báo cáo nào</p>
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
            {reports.map((r) => (
              <Row key={r.id} report={r} />
            ))}
          </tbody>
        </table>
      )}
      {nextCursor ? (
        <span className="mt-4 inline-block text-sm text-text-secondary">
          <Link
            href={`/admin/cases?tab=reports&status=${status}&cursor=${encodeURIComponent(nextCursor)}`}
            variant="subtle"
          >
            Trang sau
          </Link>
        </span>
      ) : null}
    </div>
  );
}
