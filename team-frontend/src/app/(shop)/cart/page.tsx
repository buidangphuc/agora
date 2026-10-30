import Link from "next/link";

import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Empty } from "@/components/ui/Empty";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { CartGroups } from "@/features/cart/CartGroups";
import { CartSelection } from "@/features/cart/CartSelection";
import { ClearCartButton } from "@/features/cart/ClearCartButton";
import { groupByShop } from "@/features/cart/groupByShop";
import { linkButtonPrimary } from "@/features/cart/linkButton";
import { VoucherModal } from "@/features/voucher/VoucherModal";
import { isCheckoutEnabled } from "@/lib/flags";
import { getCartWithShopNames } from "@/lib/gateway/cart";
import { listVouchers, previewVoucher } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

type SearchParams = Record<string, string | string[] | undefined>;

export default async function CartPage(props: {
  searchParams: Promise<SearchParams>;
}) {
  const searchParams = await props.searchParams;
  if (!getPrincipal()) redirectGuestToLogin("/cart", searchParams);
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
  const rawItems = searchParams.items;
  const initialItems = Array.isArray(rawItems) ? rawItems[0] : rawItems;
  const applied = preview?.valid ? code : undefined;
  const discount = preview?.valid ? preview.discountAmount : 0;
  return (
    <section className="space-y-4 py-2 pb-24 lg:pb-2">
      <div className="hidden lg:block">
        <Breadcrumb
          items={[{ label: "Trang chủ", href: "/" }, { label: "Giỏ hàng" }]}
        />
      </div>
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold text-text-primary">
          Giỏ hàng ({cart.items.length} sản phẩm)
        </h1>
        <ClearCartButton />
      </div>

      <CartSelection
        lines={cart.items.map((i) => ({
          id: i.id,
          sellerId: i.sellerId,
          unitPrice: i.unitPrice,
          quantity: i.quantity,
        }))}
        initialItems={initialItems}
        checkoutEnabled={checkoutEnabled}
        discount={discount}
        voucherCode={applied}
      >
        <CartGroups groups={groups} />
        <VoucherModal
          vouchers={vouchers}
          subtotal={cart.subtotal}
          sellerId={sellerId}
          appliedCode={applied}
          appliedDiscount={discount}
        />
      </CartSelection>
    </section>
  );
}
