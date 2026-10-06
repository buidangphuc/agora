import { Statistic } from "@/components/ui/Statistic";

export interface KpiCell {
  key: string;
  title: string;
  /** Pre-formatted value. */
  value: string;
}

export const KPI_GRID = "grid grid-cols-2 gap-4 lg:grid-cols-4";

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
    <div className={KPI_GRID} data-testid="kpi-row">
      {cells.map((c) => (
        <Statistic
          key={c.key}
          title={c.title}
          value={c.value}
          loading={loading}
        />
      ))}
    </div>
  );
}
