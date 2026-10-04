import Link from "next/link";

import { Alert } from "@/components/ui/Alert";
import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Button } from "@/components/ui/Button";
import { Empty } from "@/components/ui/Empty";
import { CartGroups } from "@/features/cart/CartGroups";
import { ClearCartButton } from "@/features/cart/ClearCartButton";
import { groupByShop } from "@/features/cart/groupByShop";
import {
  linkButtonOutline,
  linkButtonPrimary,
} from "@/features/cart/linkButton";
import { OrderSummary } from "@/features/order/OrderSummary";
import { VoucherModal } from "@/features/voucher/VoucherModal";
import { isCheckoutEnabled } from "@/lib/flags";
import { getCartWithShopNames } from "@/lib/gateway/cart";
import { listVouchers, previewVoucher } from "@/lib/gateway/promotion";

export const dynamic = "force-dynamic";

type SearchParams = Record<string, string | string[] | undefined>;

export default async function CartPage({
  searchParams,
}: {
  searchParams: SearchParams;
}) {
  // Evaluate the checkout kill-switch server-side; the browser only receives the
  // resolved boolean, never the flag SDK or the Flipt endpoint.
  const [cart, checkoutEnabled] = await Promise.all([
    getCartWithShopNames(),
    isCheckoutEnabled(),
  ]);

  if (cart.items.length === 0) {
    return (
      <section className="py-2">
        <Empty
          description="Giỏ hàng của bạn đang trống"
          action={
            <Link href="/" className={linkButtonPrimary}>
              Tiếp tục mua sắm
            </Link>
          }
        />
      </section>
    );
  }

  const groups = groupByShop(cart.items);
  // One voucher per order, validated against the first shop's items (unchanged
  // contract); the server previews the discount, the browser never computes it.
  const sellerId = cart.items[0]?.sellerId ?? "";
  const rawVoucher = searchParams.voucher;
  const code = (Array.isArray(rawVoucher) ? rawVoucher[0] : rawVoucher)?.trim();
  const [preview, vouchers] = await Promise.all([
    code ? previewVoucher(code, cart.subtotal, sellerId) : null,
    listVouchers(sellerId),
  ]);
  const applied = preview?.valid ? code : undefined;
  const discount = preview?.valid ? preview.discountAmount : 0;
  const checkoutHref = applied
    ? `/checkout?voucher=${encodeURIComponent(applied)}`
    : "/checkout";

  return (
    <section className="space-y-4 py-2 pb-24 lg:pb-2">
      <Breadcrumb
        className="hidden lg:block"
        items={[{ label: "Trang chủ", href: "/" }, { label: "Giỏ hàng" }]}
      />
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold text-text-primary">
          Giỏ hàng ({cart.items.length} sản phẩm)
        </h1>
        <ClearCartButton />
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <CartGroups groups={groups} />
          <VoucherModal
            vouchers={vouchers}
            subtotal={cart.subtotal}
            sellerId={sellerId}
            appliedCode={applied}
            appliedDiscount={discount}
          />
        </div>
        <div>
          <OrderSummary
            itemCount={cart.items.length}
            subtotal={cart.subtotal}
            discount={discount}
            voucherCode={applied}
            shipping={null}
            notice={
              checkoutEnabled ? undefined : (
                <Alert
                  type="warning"
                  description="Thanh toán tạm thời không khả dụng. Vui lòng thử lại sau."
                />
              )
            }
            action={
              checkoutEnabled ? (
                <Link
                  href={checkoutHref}
                  className={`${linkButtonPrimary} w-full`}
                >
                  Mua hàng
                </Link>
              ) : (
                <Button size="lg" disabled className="w-full">
                  Mua hàng
                </Button>
              )
            }
            mobileAction={
              checkoutEnabled ? (
                <Link href={checkoutHref} className={linkButtonPrimary}>
                  Mua hàng
                </Link>
              ) : (
                <Button size="lg" disabled>
                  Mua hàng
                </Button>
              )
            }
          />
        </div>
      </div>
    </section>
  );
}
