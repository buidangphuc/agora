import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

const ROWS = ["row-1", "row-2", "row-3"];

/** Same footprint as /cart: header, one shop card with 3 rows, summary card. */
export default function CartLoading() {
  return (
    <section
      className="space-y-6 py-4 pb-24 sm:py-6 lg:pb-8"
      aria-busy="true"
      data-testid="cart-skeleton"
    >
      <div className="flex items-center justify-between gap-3">
        <div className="w-56">
          <Skeleton variant="text" lines={1} />
        </div>
      </div>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3 lg:gap-8">
        <div className="space-y-6 lg:col-span-2">
          <Card>
            <CardHeader>
              <div className="w-40">
                <Skeleton variant="text" lines={1} />
              </div>
            </CardHeader>
            <ul className="divide-y divide-border-subtle">
              {ROWS.map((row) => (
                <li
                  key={row}
                  className="flex items-center gap-4 px-4 py-5 sm:gap-6 sm:px-6"
                >
                  <div className="w-20 shrink-0 sm:w-24">
                    <Skeleton variant="image" />
                  </div>
                  <div className="flex-1">
                    <Skeleton variant="text" lines={2} />
                  </div>
                  <div className="hidden w-28 sm:block">
                    <Skeleton variant="text" lines={1} />
                  </div>
                </li>
              ))}
            </ul>
          </Card>
        </div>
        <div className="hidden lg:block">
          <Card>
            <CardHeader>
              <div className="w-32">
                <Skeleton variant="text" lines={1} />
              </div>
            </CardHeader>
            <CardContent>
              <Skeleton variant="text" lines={4} />
            </CardContent>
          </Card>
        </div>
      </div>
    </section>
  );
}
