import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

/** Same footprint as a checkout step: step panel + summary card (the shell header streams separately). */
export default function CheckoutLoading() {
  return (
    <section
      className="py-2 pb-24 lg:pb-2"
      aria-busy="true"
      data-testid="checkout-skeleton"
    >
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <Card>
            <CardHeader>
              <Skeleton variant="text" lines={1} className="w-40" />
            </CardHeader>
            <CardContent>
              <Skeleton variant="text" lines={5} />
            </CardContent>
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
