import { Card } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";
import { range } from "@/components/ui/range";

const block = "animate-pulse bg-neutral-200";

/** Route skeleton for `/vouchers`: banner, tabs and a grid of voucher cards (1 column on mobile, 2 from lg). */
export default function VouchersLoading() {
  return (
    <section className="space-y-6 py-2" aria-busy="true">
      <div className={`h-32 rounded-2xl ${block}`} />
      <div className="flex max-w-full gap-6 overflow-x-auto border-b border-border-subtle pb-3">
        {range(4).map((i) => (
          <div key={i} className={`h-5 w-20 shrink-0 rounded-xs ${block}`} />
        ))}
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {range(4).map((i) => (
          <Card key={i} className="p-4">
            <Skeleton variant="text" lines={4} />
          </Card>
        ))}
      </div>
    </section>
  );
}
