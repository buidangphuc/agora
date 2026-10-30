import { Breadcrumb } from "@/components/ui/Breadcrumb";
import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Link } from "@/components/ui/Link";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { CartGroups } from "@/features/cart/CartGroups";
import { CartSelection } from "@/features/cart/CartSelection";
import { ClearCartButton } from "@/features/cart/ClearCartButton";
import { groupByShop } from "@/features/cart/groupByShop";
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
      <section className="mx-auto w-full max-w-xl py-6 sm:py-10">
        <Card padded>
          <Empty
            description="Giỏ hàng của bạn đang trống"
            action={
              <Link href="/" variant="primary" size="lg">
                Tiếp tục mua sắm
              </Link>
            }
          />
        </Card>
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
    <section className="space-y-6 py-4 pb-24 sm:py-6 lg:pb-8">
      <Breadcrumb
        className="hidden lg:block"
        items={[{ label: "Trang chủ", href: "/" }, { label: "Giỏ hàng" }]}
      />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary sm:text-3xl">
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
