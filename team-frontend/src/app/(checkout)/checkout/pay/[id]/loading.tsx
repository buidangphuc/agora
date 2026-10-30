import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

/** Same footprint as the payment page: summary card + simulator card. */
export default function PaymentLoading() {
  return (
    <section
      className="mx-auto w-full min-w-0 max-w-xl space-y-4 md:space-y-6"
      aria-busy="true"
      data-testid="payment-skeleton"
    >
      <Card>
        <CardHeader>
          <Skeleton variant="text" lines={1} className="w-40" />
        </CardHeader>
        <CardContent>
          <Skeleton variant="text" lines={3} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <Skeleton variant="text" lines={1} className="w-40" />
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <Skeleton variant="text" lines={1} />
            <Skeleton variant="text" lines={1} />
          </div>
        </CardContent>
      </Card>
    </section>
  );
}
