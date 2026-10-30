import NextLink from "next/link";

import { Link } from "@/components/ui/Link";
import { focusRing } from "@/components/ui/focus";

/**
 * Home hero: one primary call to action and one quiet secondary link. No
 * campaign figures (discounts, cashback, instalments): none come from a backend.
 * There is no hero picture asset, so the banner is a token gradient with a
 * fixed height (static markup, nothing loads, nothing shifts).
 */
export function Hero() {
  return (
    <section
      aria-labelledby="home-hero-title"
      className="relative flex min-h-72 flex-col justify-center overflow-hidden rounded-3xl bg-gradient-to-br from-primary-500 to-primary-700 px-6 py-10 text-text-inverse shadow-card sm:min-h-80 sm:px-10 lg:px-14"
    >
      <div className="max-w-2xl space-y-5">
        <h1
          id="home-hero-title"
          className="text-2xl font-bold leading-tight tracking-tight sm:text-4xl"
        >
          Mua sắm trực tuyến từ các shop trên Marketplace
        </h1>
        <p className="text-base text-primary-100 sm:text-lg">
          Tìm sản phẩm, so sánh giá và đặt hàng chỉ trong vài bước.
        </p>
        <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center">
          <Link href="/search" variant="white" size="md" fullWidth="mobile">
            Mua ngay
          </Link>
          <NextLink
            href="/vouchers"
            className={`inline-flex min-h-touch items-center justify-center rounded-xl px-4 text-base font-medium text-text-inverse underline-offset-4 hover:underline ${focusRing}`}
          >
            Xem kho voucher
          </NextLink>
        </div>
      </div>
    </section>
  );
}
