import type { ConsentEntry } from "@/lib/gateway/privacy";

const LABELS: Record<string, string> = {
  ANALYTICS: "Phân tích",
  MARKETING: "Tiếp thị",
  PERSONALIZATION: "Cá nhân hóa",
  TERMS: "Điều khoản sử dụng",
  PRIVACY_POLICY: "Chính sách quyền riêng tư",
  SELLER_TERMS: "Điều khoản người bán",
};

export function ConsentHistory({ records }: { records: ConsentEntry[] }) {
  if (records.length === 0) {
    return (
      <p className="text-sm text-text-secondary">Chưa có lịch sử đồng ý.</p>
    );
  }
  return (
    <ul className="flex flex-col gap-2 text-sm">
      {records.map((r, i) => (
        <li key={`${r.purpose}-${i}`} className="flex flex-wrap gap-2">
          <span>{LABELS[r.purpose] ?? r.purpose}</span>
          <span>{r.granted ? "Đồng ý" : "Từ chối"}</span>
          <span className="text-text-secondary">Phiên bản {r.version}</span>
        </li>
      ))}
    </ul>
  );
}
