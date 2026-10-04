import Link from "next/link";

import { focusRing } from "@/components/ui/focus";
import { linkButtonClass } from "./linkButton";

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
      className="relative flex min-h-60 flex-col justify-center overflow-hidden rounded-2xl bg-gradient-to-r from-primary-500 to-primary-700 p-6 text-text-inverse shadow-preline-card sm:min-h-64 sm:p-8"
    >
      <div className="max-w-xl space-y-4">
        <h1
          id="home-hero-title"
          className="text-lg font-bold leading-snug sm:text-2xl"
        >
          Mua sắm trực tuyến từ các shop trên Marketplace
        </h1>
        <p className="text-sm text-primary-100">
          Tìm sản phẩm, so sánh giá và đặt hàng chỉ trong vài bước.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Link href="/search" className={linkButtonClass("white")}>
            Mua ngay
          </Link>
          <Link
            href="/vouchers"
            className={`inline-flex min-h-11 items-center rounded-lg px-2 text-sm font-medium text-text-inverse underline-offset-4 hover:underline ${focusRing}`}
          >
            Xem kho voucher
          </Link>
        </div>
      </div>
    </section>
  );
}
