import { Statistic } from "@/components/ui/Statistic";

export interface KpiCell {
  key: string;
  title: string;
  /** Pre-formatted value. */
  value: string;
  note?: string;
  tone?: "danger";
}

export const KPI_GRID =
  "grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4 lg:gap-6";

/**
 * Responsive KPI row: 2-up on mobile, 4-up on desktop. The skeleton
 * (`loading`) renders the same Statistic cells, so the row keeps its height
 * (CLS 0). Pass only the KPIs that have a real source.
 */
export function KpiRow({
  cells,
  loading = false,
}: { cells: KpiCell[]; loading?: boolean }) {
  return (
    <div
      className={KPI_GRID}
      // The skeleton gets its own id: React 19 reveals a streamed boundary a
      // beat after its content lands (hidden), so both rows can co-exist.
      data-testid={loading ? "kpi-row-skeleton" : "kpi-row"}
    >
      {cells.map((c) => (
        <Statistic
          key={c.key}
          title={c.title}
          value={c.value}
          loading={loading}
          suffix={c.note}
          tone={c.tone}
        />
      ))}
    </div>
  );
}
