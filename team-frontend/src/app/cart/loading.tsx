import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

const ROWS = ["row-1", "row-2", "row-3"];

/** Same footprint as /cart: header, one shop card with 3 rows, summary card. */
export default function CartLoading() {
  return (
    <section
      className="space-y-4 py-2 pb-24 lg:pb-2"
      aria-busy="true"
      data-testid="cart-skeleton"
    >
      <div className="flex items-center justify-between gap-3">
        <Skeleton variant="text" lines={1} className="w-48" />
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <Card>
            <CardHeader>
              <Skeleton variant="text" lines={1} className="w-40" />
            </CardHeader>
            <ul className="divide-y divide-border-subtle">
              {ROWS.map((row) => (
                <li key={row} className="flex items-center gap-3 p-4">
                  <Skeleton variant="image" className="w-20 shrink-0" />
                  <Skeleton variant="text" lines={2} className="flex-1" />
                  <Skeleton variant="text" lines={1} className="w-28" />
                </li>
              ))}
            </ul>
          </Card>
        </div>
        <Card className="hidden lg:block">
          <CardHeader>
            <Skeleton variant="text" lines={1} className="w-32" />
          </CardHeader>
          <CardContent>
            <Skeleton variant="text" lines={4} />
          </CardContent>
        </Card>
      </div>
    </section>
  );
}
