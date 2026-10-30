import Link from "next/link";

import type { CasesTab } from "./casesParams";

export const CASES_TABS: { key: CasesTab; label: string; href: string }[] = [
  { key: "reports", label: "Báo cáo", href: "/admin/cases?tab=reports" },
  { key: "queue", label: "Hàng chờ duyệt", href: "/admin/cases?tab=queue" },
];

export function AdminCasesTabs({ active }: { active: CasesTab }) {
  return (
    <nav aria-label="Hồ sơ cần xử lý" className="flex gap-4 border-b">
      {CASES_TABS.map((t) => (
        <Link
          key={t.key}
          href={t.href}
          aria-current={t.key === active ? "page" : undefined}
          className={`px-2 py-2 text-sm ${t.key === active ? "border-b-2 font-semibold text-text-primary" : "text-text-secondary"}`}
        >
          {t.label}
        </Link>
      ))}
    </nav>
  );
}
