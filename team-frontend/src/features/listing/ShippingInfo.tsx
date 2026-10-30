import { formatPrice } from "@/components/ui/format";
import { formatDeliveryDate } from "@/features/order/deliveryDate";
import {
  FREE_SHIPPING_THRESHOLD,
  SHIPPING_FEE_RANGE,
} from "@/features/order/shippingCopy";
import { listAddresses } from "@/lib/gateway/addresses";
import { calculateShippingFee } from "@/lib/gateway/orders";

function ShippingRow({ children }: { children: React.ReactNode }) {
  return (
    <dl className="text-sm">
      <div className="flex flex-col gap-1 sm:flex-row sm:gap-4">
        <dt className="shrink-0 font-semibold text-text-primary sm:w-24 sm:font-normal sm:text-text-secondary">
          Vận chuyển
        </dt>
        <dd className="min-w-0 leading-relaxed text-text-primary">
          {children}
        </dd>
      </div>
    </dl>
  );
}

const rangeText = (
  <>
    Phí vận chuyển từ {formatPrice(SHIPPING_FEE_RANGE.min)} đến{" "}
    {formatPrice(SHIPPING_FEE_RANGE.max)}. Miễn phí cho đơn từ{" "}
    {formatPrice(FREE_SHIPPING_THRESHOLD)}.
  </>
);

/** Suspense fallback while the buyer's city fee loads: the same range copy. */
export function ShippingInfoFallback() {
  return <ShippingRow>{rangeText}</ShippingRow>;
}

/**
 * PDP shipping row. Signed in with a default address: that city's fee from
 * team-order. Anonymous, no default address or a failed quote: the range.
 */
export async function ShippingInfo({ signedIn }: { signedIn: boolean }) {
  let city: string | null = null;
  let fee: number | null = null;
  let date: string | null = null;
  if (signedIn) {
    const addresses = await listAddresses();
    const def = addresses.find((a) => a.isDefault);
    if (def) {
      const quote = await calculateShippingFee(def.city, 0);
      if (quote) {
        city = def.city;
        fee = quote.shippingFee;
        date = formatDeliveryDate(quote.estimatedDeliveryAt);
      }
    }
  }
  if (fee === null) return <ShippingInfoFallback />;
  return (
    <ShippingRow>
      Phí vận chuyển đến {city}: {formatPrice(fee)}. Miễn phí cho đơn từ{" "}
      {formatPrice(FREE_SHIPPING_THRESHOLD)}.
      {date ? ` Nhận hàng dự kiến ${date}.` : null}
    </ShippingRow>
  );
}
