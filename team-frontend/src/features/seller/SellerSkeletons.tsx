import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { KpiRow } from "./KpiRow";

/** Same footprint as SellerPageHeader: breadcrumb line, title, description. */
export function HeaderSkeleton() {
  return (
    <div className="space-y-3">
      <Skeleton variant="text" lines={1} className="w-40" />
      <Skeleton variant="text" lines={1} className="w-64" />
      <Skeleton variant="text" lines={1} className="w-96 max-w-full" />
    </div>
  );
}

const KPI_PLACEHOLDERS = ["k1", "k2", "k3", "k4"];

/** The KPI row in its loading state: the very same cells as the page. */
export function KpiRowSkeleton({ count = 4 }: { count?: number }) {
  return (
    <KpiRow
      loading
      cells={KPI_PLACEHOLDERS.slice(0, count).map((key) => ({
        key,
        title: "",
        value: "",
      }))}
    />
  );
}

/** A Card holding a filter bar and table rows. */
export function TableCardSkeleton({
  rows = 5,
  filter = true,
}: { rows?: number; filter?: boolean }) {
  return (
    <Card aria-busy="true">
      <CardHeader>
        <Skeleton variant="text" lines={1} className="w-40" />
      </CardHeader>
      <CardContent className="space-y-4">
        {filter && <Skeleton variant="text" lines={1} className="h-10" />}
        <Skeleton variant="text" lines={rows} />
      </CardContent>
    </Card>
  );
}

/** A Card holding a form-like block. */
export function FormCardSkeleton({ lines = 5 }: { lines?: number }) {
  return (
    <Card aria-busy="true">
      <CardContent>
        <Skeleton variant="text" lines={lines} />
      </CardContent>
    </Card>
  );
}
