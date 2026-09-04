import Link from "next/link";

import { formatPrice } from "@/components/ui/format";
import { AiAssistantModal } from "@/features/ai/AiAssistantModal";
import { LoyaltyWidget } from "@/features/engagement/LoyaltyWidget";
import { RecentlyViewedRow } from "@/features/home/RecentlyViewedRow";
import { ListingGrid } from "@/features/listing/ListingGrid";
import { RecommendationsRow } from "@/features/recommendations/RecommendationsRow";
import { getLoyalty } from "@/lib/gateway/engagement";
import { listCategories, listListings } from "@/lib/gateway/listings";
import { getPrincipal } from "@/lib/gateway/session";
import { getImageUrl } from "@/lib/media";

export const dynamic = "force-dynamic";

const SERVICE_HUBS = [
  {
    name: "Khung Giờ Săn Sale",
    icon: "⚡",
    href: "/#flash-sale",
    badge: "Hot",
    bg: "bg-amber-50 text-amber-600 border-amber-200",
  },
  {
    name: "Miễn Phí Vận Chuyển",
    icon: "🚚",
    href: "/vouchers",
    badge: "0Đ",
    bg: "bg-emerald-50 text-emerald-600 border-emerald-200",
  },
  {
    name: "Mã Giảm Giá 100k",
    icon: "🎟️",
    href: "/vouchers",
    badge: "Voucher",
    bg: "bg-orange-50 text-brand border-orange-200",
  },
  {
    name: "Hàng Chính Hãng",
    icon: "🏷️",
    href: "/search?q=chính+hãng",
    badge: "Auth 100%",
    bg: "bg-red-50 text-red-600 border-red-200",
  },
  {
    name: "Nạp Thẻ & Hóa Đơn",
    icon: "📱",
    href: "/search",
    badge: "-5%",
    bg: "bg-blue-50 text-blue-600 border-blue-200",
  },
  {
    name: "Tích Xu Đổi Quà",
    icon: "🪙",
    href: "/search",
    badge: "Thưởng",
    bg: "bg-yellow-50 text-yellow-600 border-yellow-200",
  },
  {
    name: "Hàng Quốc Tế",
    icon: "✈️",
    href: "/search",
    badge: "Freeship",
    bg: "bg-purple-50 text-purple-600 border-purple-200",
  },
  {
    name: "Deal Siêu Rẻ",
    icon: "💰",
    href: "/#flash-sale",
    badge: "Từ 1k",
    bg: "bg-rose-50 text-rose-600 border-rose-200",
  },
];

export default async function HomePage() {
  const [categories, page] = await Promise.all([
    listCategories().catch(() => []),
    listListings({ status: "published", pageSize: 24 }).catch(() => ({
      items: [],
      nextCursor: "",
      total: 0,
    })),
  ]);

  const principal = getPrincipal();
  const loyalty = principal ? await getLoyalty() : null;

  const items = page.items;
  const flashSaleItems = items.slice(0, 6);
  const mallItems = items.filter(
    (l) =>
      l.price > 5000000 ||
      l.title.toLowerCase().includes("chính hãng") ||
      l.title.toLowerCase().includes("apple") ||
      l.title.toLowerCase().includes("sony") ||
      l.title.toLowerCase().includes("philips") ||
      l.title.toLowerCase().includes("nike"),
  );

  return (
    <div className="space-y-4">
      {/* ── Loyalty daily check-in (logged-in only) ── */}
      {loyalty && <LoyaltyWidget initial={loyalty} />}

      {/* ── 1. Hero Promotional Banner Carousel ── */}
      <section className="grid grid-cols-1 md:grid-cols-12 gap-1.5 rounded-xs overflow-hidden">
        {/* Left: Main Big Promotional Banner */}
        <div className="md:col-span-8 relative aspect-2/1 overflow-hidden rounded-xs bg-gradient-to-r from-[#ee4d2d] via-[#f53d2d] to-[#d0011b] text-white p-7 sm:p-8 flex flex-col justify-between shadow-shopee">
          {/* Decorative background circle */}
          <div className="absolute -right-10 -bottom-10 w-64 h-64 rounded-full bg-white/10 blur-2xl pointer-events-none" />

          <div className="relative z-10 max-w-lg space-y-2">
            <div className="inline-flex items-center gap-1.5 rounded-full bg-[#ffe97a] px-3 py-0.5 text-[11px] font-black text-[#ee4d2d] uppercase tracking-wider shadow-xs">
              <span>★</span>
              <span>SIÊU HỘI MUA SẮM POLYREPO 2026</span>
            </div>
            <h1 className="text-2xl sm:text-4xl font-black leading-tight tracking-tight drop-shadow-sm">
              SĂN SALE CÔNG NGHỆ & THỜI TRANG{" "}
              <br className="hidden sm:inline" />
              <span className="text-[#ffe97a]">GIẢM ĐẾN 50%</span>
            </h1>
            <p className="text-xs sm:text-sm text-orange-100 font-normal">
              Voucher Freeship 0Đ Toàn Quốc · Hoàn Xu 20% Đơn Hàng · Trả Góp 0%
            </p>
          </div>

          <div className="relative z-10 flex gap-3 pt-3">
            <Link
              href="/search"
              className="rounded-xs bg-[#ffe97a] px-7 py-2.5 text-xs font-bold text-gray-900 shadow-md hover:bg-yellow-300 transition uppercase tracking-wider"
            >
              MUA NGAY
            </Link>
            <Link
              href="/vouchers"
              className="rounded-xs bg-white/20 px-6 py-2.5 text-xs font-bold text-white backdrop-blur-xs hover:bg-white/30 transition uppercase tracking-wider border border-white/40"
            >
              LƯU VOUCHER 100K
            </Link>
          </div>
        </div>

        {/* Right: 2 Stacked Mini Promotional Banners */}
        <div className="md:col-span-4 grid grid-cols-2 md:grid-cols-1 gap-1.5">
          <div className="rounded-xs bg-gradient-to-r from-[#d0011b] to-[#ee4d2d] p-4 text-white flex flex-col justify-between shadow-shopee relative overflow-hidden">
            <div className="relative z-10">
              <span className="text-[10px] font-extrabold uppercase tracking-wider text-[#ffe97a]">
                Top Lựa Chọn
              </span>
              <h3 className="font-black text-sm mt-0.5 leading-snug">
                Hàng Chọn Giá Tốt Từ 1k
              </h3>
              <p className="text-[11px] text-orange-100 mt-0.5">
                Tuyển chọn hàng rẻ vô địch
              </p>
            </div>
            <Link
              href="/search?q=choice"
              className="relative z-10 text-[11px] font-bold text-[#ffe97a] hover:underline mt-2 flex items-center gap-1"
            >
              <span>Khám phá ngay</span>
              <span>→</span>
            </Link>
          </div>

          <div className="rounded-xs bg-gradient-to-r from-amber-500 to-[#ee4d2d] p-4 text-white flex flex-col justify-between shadow-shopee relative overflow-hidden">
            <div className="relative z-10">
              <span className="text-[10px] font-extrabold uppercase tracking-wider text-white/90">
                Siêu Hội Hoàn Xu
              </span>
              <h3 className="font-black text-sm mt-0.5 leading-snug">
                Tích Xu Đổi Quà 500k
              </h3>
              <p className="text-[11px] text-orange-100 mt-0.5">
                Hoàn xu cực đã mỗi ngày
              </p>
            </div>
            <Link
              href="/vouchers"
              className="relative z-10 text-[11px] font-bold text-[#ffe97a] hover:underline mt-2 flex items-center gap-1"
            >
              <span>Thu thập ngay</span>
              <span>→</span>
            </Link>
          </div>
        </div>
      </section>

      {/* ── 2. Shopee Quick Service Hubs (8 Iconic Circles) ── */}
      <section className="rounded-xs bg-white p-4 shadow-shopee border border-gray-100">
        <div className="grid grid-cols-4 sm:grid-cols-8 gap-3 text-center">
          {SERVICE_HUBS.map((hub) => (
            <Link
              key={hub.name}
              href={hub.href}
              className="flex flex-col items-center justify-center p-1.5 rounded-xs hover:bg-orange-50/40 transition group"
            >
              <div
                className={`relative grid h-12 w-12 place-items-center rounded-2xl ${hub.bg} text-2xl group-hover:scale-110 transition duration-200 border shadow-2xs`}
              >
                <span>{hub.icon}</span>
                {hub.badge && (
                  <span className="absolute -top-2 -right-2 rounded-full bg-red-600 px-1.5 py-0.2 text-[8px] font-black text-white shadow-2xs">
                    {hub.badge}
                  </span>
                )}
              </div>
              <span className="mt-2 text-[12px] font-normal text-gray-800 line-clamp-1 group-hover:text-brand transition">
                {hub.name}
              </span>
            </Link>
          ))}
        </div>
      </section>

      {/* ── 3. Danh Mục Ngành Hàng (Category Grid) ── */}
      <section className="rounded-xs bg-white shadow-shopee border border-gray-100">
        <div className="border-b border-gray-100 px-5 py-3 text-xs font-bold uppercase tracking-wider text-gray-600 flex items-center justify-between">
          <span>DANH MỤC NGÀNH HÀNG</span>
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-5 md:grid-cols-10 divide-x divide-y divide-gray-100">
          {categories.map((cat) => (
            <Link
              key={cat.id}
              href={`/search?category=${cat.id}`}
              className="flex flex-col items-center justify-center p-3 text-center hover:shadow-xs hover:border-brand transition group bg-white"
            >
              <span className="text-3xl group-hover:scale-110 transition duration-200">
                {cat.iconUrl || "🛍️"}
              </span>
              <span className="mt-2 text-[12px] font-normal text-gray-700 group-hover:text-brand line-clamp-2 leading-snug">
                {cat.name}
              </span>
            </Link>
          ))}
        </div>
      </section>

      {/* ── 4. ⚡ FLASH SALE SECTION ── */}
      <section
        id="flash-sale"
        className="scroll-mt-24 rounded-xs bg-white p-4 shadow-shopee border border-gray-100 space-y-3"
      >
        <div className="flex items-center justify-between border-b border-gray-100 pb-2.5">
          <div className="flex items-center gap-3">
            <span className="font-black text-lg text-brand uppercase tracking-tighter flex items-center gap-1.5">
              <span>⚡</span>
              <span>FLASH SALE</span>
            </span>
            <div className="flex items-center gap-1 text-xs font-bold text-white">
              <span className="rounded-2xs bg-black px-1.5 py-0.5">02</span>
              <span className="text-black font-bold">:</span>
              <span className="rounded-2xs bg-black px-1.5 py-0.5">15</span>
              <span className="text-black font-bold">:</span>
              <span className="rounded-2xs bg-black px-1.5 py-0.5">48</span>
            </div>
          </div>
          <Link
            href="/#flash-sale"
            className="text-xs font-medium text-brand hover:underline"
          >
            Xem tất cả &gt;
          </Link>
        </div>

        {/* Flash Sale 6-Item Row */}
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-6 gap-2.5">
          {flashSaleItems.map((item) => {
            const imageSrc =
              item.imageKeys && item.imageKeys.length > 0
                ? getImageUrl(item.imageKeys[0])
                : item.imageUrl;

            return (
              <Link
                key={item.id}
                href={`/listing/${item.id}`}
                className="group flex flex-col items-center text-center p-2 rounded-xs hover:shadow-shopee-hover transition bg-white border border-gray-100"
              >
                <div className="relative aspect-square w-full overflow-hidden bg-gray-50 rounded-xs">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={imageSrc}
                    alt={item.title}
                    className="h-full w-full object-cover group-hover:scale-105 transition duration-300"
                  />
                  <div className="absolute top-0 right-0 bg-[#ffe97a] px-1 py-0.2 text-[9px] font-black text-brand">
                    -25%
                  </div>
                </div>

                <div className="mt-2.5 w-full">
                  <p className="text-sm font-semibold text-brand">
                    {formatPrice(item.price)}
                  </p>

                  {/* Flame progress bar */}
                  <div className="relative mt-1.5 h-3.5 w-full rounded-full bg-orange-100 overflow-hidden text-[9px] font-black text-white flex items-center justify-center">
                    <div
                      className="absolute left-0 top-0 h-full bg-gradient-to-r from-red-600 to-orange-500 rounded-full"
                      style={{ width: "82%" }}
                    />
                    <span className="relative z-10 text-[8px] uppercase tracking-wider">
                      🔥 ĐÃ BÁN 82%
                    </span>
                  </div>
                </div>
              </Link>
            );
          })}
        </div>
      </section>

      {/* ── 5. THƯƠNG HIỆU CHÍNH HÃNG (Official Brands) ── */}
      {mallItems.length > 0 && (
        <section className="rounded-xs bg-white p-4 shadow-shopee border border-gray-100 space-y-3">
          <div className="flex items-center justify-between border-b border-gray-100 pb-2.5">
            <div className="flex items-center gap-3">
              <span className="font-black text-sm sm:text-base text-[#d0011b] uppercase tracking-wider">
                THƯƠNG HIỆU CHÍNH HÃNG
              </span>
              <div className="hidden sm:flex items-center gap-4 text-xs text-gray-500">
                <span>✓ 7 Ngày Miễn Phí Trả Hàng</span>
                <span>✓ Hàng Chính Hãng 100%</span>
                <span>✓ Miễn Phí Vận Chuyển</span>
              </div>
            </div>
            <Link
              href="/search?q=mall"
              className="text-xs font-medium text-[#d0011b] hover:underline"
            >
              Xem tất cả &gt;
            </Link>
          </div>

          <ListingGrid
            listings={mallItems.slice(0, 6)}
            empty="Chưa có sản phẩm chính hãng nào."
          />
        </section>
      )}

      {/* ── 6. GỢI Ý HÔM NAY (AI Recommendation Feed) ── */}
      <section className="space-y-3">
        {/* Sticky Tab Header */}
        <div className="rounded-xs bg-white p-3 shadow-shopee border-b-2 border-brand flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="font-bold text-sm text-brand uppercase tracking-wider flex items-center gap-1.5">
              <span>🤖</span>
              <span>GỢI Ý HÔM NAY (AI RECOMMENDATION)</span>
            </span>
          </div>
          <span className="text-[12px] text-gray-400 font-normal">
            Cá nhân hóa theo sở thích & nhu cầu mua sắm
          </span>
        </div>

        {/* High-density 6-column Grid */}
        <ListingGrid
          listings={items}
          empty="Hiện chưa có sản phẩm nào được đăng bán."
        />

        {/* Load More Button */}
        <div className="pt-4 text-center">
          <Link
            href="/search"
            className="inline-block rounded-xs border border-gray-300 bg-white px-12 py-2.5 text-xs font-medium text-gray-700 shadow-shopee hover:bg-gray-50 hover:border-brand hover:text-brand transition uppercase tracking-wider"
          >
            Xem Thêm Gợi Ý
          </Link>
        </div>
      </section>

      {/* ── "Vừa xem" — recently-viewed listings from team-engagement
             (GetRecentlyViewed) via the gateway. Hidden for anonymous users or
             when there's no view history. ── */}
      <RecentlyViewedRow />

      {/* ── 7. "Gợi ý cho bạn" — personalized recommendations from team-ai
             (RecommendationService/Recommend) via the gateway. Hidden when the
             service is unavailable. ── */}
      <RecommendationsRow />

      {/* ── 8. Floating AI Assistant Component ── */}
      <AiAssistantModal listings={items} />
    </div>
  );
}
