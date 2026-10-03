import Link from "next/link";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { PriceTag } from "@/components/ui/PriceTag";
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
    bg: "bg-orange-50 text-primary-600 border-orange-200",
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
    <div className="space-y-6">
      {/* ── Loyalty daily check-in (logged-in only) ── */}
      {loyalty && <LoyaltyWidget initial={loyalty} />}

      {/* ── 1. Hero Promotional Banner Grid ── */}
      <section className="grid grid-cols-1 md:grid-cols-12 gap-3">
        {/* Left: Main Big Promotional Banner */}
        <div className="md:col-span-8 relative aspect-2/1 overflow-hidden rounded-2xl bg-gradient-to-r from-primary-500 via-primary-600 to-red-600 text-white p-7 sm:p-9 flex flex-col justify-between shadow-preline-card">
          {/* Decorative background glow */}
          <div className="absolute -right-12 -bottom-12 w-72 h-72 rounded-full bg-white/10 blur-3xl pointer-events-none" />

          <div className="relative z-10 max-w-lg space-y-2.5">
            <div className="inline-flex items-center gap-1.5 rounded-full bg-amber-300 px-3 py-0.5 text-xs font-black text-primary-700 uppercase tracking-wider shadow-xs">
              <span>★</span>
              <span>SIÊU HỘI MUA SẮM POLYREPO 2026</span>
            </div>
            <h1 className="text-2xl sm:text-4xl font-black leading-tight tracking-tight drop-shadow-xs">
              SĂN SALE CÔNG NGHỆ & THỜI TRANG{" "}
              <br className="hidden sm:inline" />
              <span className="text-amber-200">GIẢM ĐẾN 50%</span>
            </h1>
            <p className="text-xs sm:text-sm text-orange-100 font-normal">
              Voucher Freeship 0Đ Toàn Quốc · Hoàn Xu 20% Đơn Hàng · Trả Góp 0%
            </p>
          </div>

          <div className="relative z-10 flex gap-3 pt-3">
            <Link href="/search">
              <Button
                variant="white"
                size="md"
                className="font-bold text-gray-900 shadow-md uppercase tracking-wider hover:bg-amber-300"
              >
                MUA NGAY
              </Button>
            </Link>
            <Link href="/vouchers">
              <Button
                variant="outline"
                size="md"
                className="bg-white/20 border-white/40 text-white hover:bg-white/30 backdrop-blur-xs uppercase tracking-wider font-bold"
              >
                LƯU VOUCHER 100K
              </Button>
            </Link>
          </div>
        </div>

        {/* Right: 2 Stacked Mini Promotional Banners */}
        <div className="md:col-span-4 grid grid-cols-2 md:grid-cols-1 gap-3">
          <div className="rounded-2xl bg-gradient-to-r from-red-600 to-primary-500 p-5 text-white flex flex-col justify-between shadow-preline-card relative overflow-hidden">
            <div className="relative z-10">
              <Badge variant="discount" size="xs">
                TOP LỰA CHỌN
              </Badge>
              <h3 className="font-bold text-base mt-2 leading-snug">
                Hàng Chọn Giá Tốt Từ 1k
              </h3>
              <p className="text-xs text-orange-100 mt-0.5">
                Tuyển chọn hàng rẻ vô địch hôm nay
              </p>
            </div>
            <Link
              href="/search?q=choice"
              className="relative z-10 text-xs font-semibold text-amber-200 hover:underline mt-3 flex items-center gap-1"
            >
              <span>Khám phá ngay</span>
              <span>→</span>
            </Link>
          </div>

          <div className="rounded-2xl bg-gradient-to-r from-amber-500 to-primary-500 p-5 text-white flex flex-col justify-between shadow-preline-card relative overflow-hidden">
            <div className="relative z-10">
              <span className="text-xs font-extrabold uppercase tracking-wider text-white/90">
                Siêu Hội Hoàn Xu
              </span>
              <h3 className="font-bold text-base mt-1 leading-snug">
                Tích Xu Đổi Quà 500k
              </h3>
              <p className="text-xs text-orange-100 mt-0.5">
                Hoàn xu cực đã mỗi ngày
              </p>
            </div>
            <Link
              href="/vouchers"
              className="relative z-10 text-xs font-semibold text-amber-200 hover:underline mt-3 flex items-center gap-1"
            >
              <span>Thu thập ngay</span>
              <span>→</span>
            </Link>
          </div>
        </div>
      </section>

      {/* ── 2. Quick Service Hubs ── */}
      <section className="rounded-2xl bg-white p-5 shadow-preline-card border border-gray-200/80">
        <div className="grid grid-cols-4 sm:grid-cols-8 gap-3 text-center">
          {SERVICE_HUBS.map((hub) => (
            <Link
              key={hub.name}
              href={hub.href}
              className="flex flex-col items-center justify-center p-2 rounded-xl hover:bg-orange-50/50 transition group"
            >
              <div
                className={`relative grid h-12 w-12 place-items-center rounded-2xl ${hub.bg} text-2xl group-hover:scale-110 transition duration-200 border shadow-2xs`}
              >
                <span>{hub.icon}</span>
                {hub.badge && (
                  <span className="absolute -top-1.5 -right-2 rounded-full bg-red-600 px-1.5 py-0.2 text-xs font-black text-white shadow-2xs">
                    {hub.badge}
                  </span>
                )}
              </div>
              <span className="mt-2 text-xs font-medium text-gray-700 line-clamp-1 group-hover:text-primary-600 transition">
                {hub.name}
              </span>
            </Link>
          ))}
        </div>
      </section>

      {/* ── 3. Danh Mục Ngành Hàng (Category Grid) ── */}
      <Card className="rounded-2xl border-gray-200/80">
        <div className="border-b border-gray-100 px-6 py-4 flex items-center justify-between">
          <span className="text-xs font-bold uppercase tracking-wider text-gray-700">
            DANH MỤC NGÀNH HÀNG
          </span>
        </div>
        <div className="grid grid-cols-2 sm:grid-cols-5 md:grid-cols-10 divide-x divide-y divide-gray-100">
          {categories.map((cat) => (
            <Link
              key={cat.id}
              href={`/search?category=${cat.id}`}
              className="flex flex-col items-center justify-center p-4 text-center hover:bg-gray-50/80 transition group bg-white"
            >
              <span className="text-3xl group-hover:scale-110 transition duration-200">
                {cat.iconUrl || "🛍️"}
              </span>
              <span className="mt-2 text-xs font-medium text-gray-700 group-hover:text-primary-600 line-clamp-2 leading-snug">
                {cat.name}
              </span>
            </Link>
          ))}
        </div>
      </Card>

      {/* ── 4. ⚡ FLASH SALE SECTION ── */}
      <Card
        id="flash-sale"
        className="scroll-mt-24 rounded-2xl border-gray-200/80 p-5 space-y-4"
      >
        <div className="flex items-center justify-between border-b border-gray-100 pb-3">
          <div className="flex items-center gap-3">
            <span className="font-black text-lg text-primary-500 uppercase tracking-tighter flex items-center gap-1.5">
              <span>⚡</span>
              <span>FLASH SALE</span>
            </span>
            <div className="flex items-center gap-1 text-xs font-bold text-white">
              <span className="rounded-md bg-gray-900 px-1.5 py-0.5">02</span>
              <span className="text-gray-900 font-bold">:</span>
              <span className="rounded-md bg-gray-900 px-1.5 py-0.5">15</span>
              <span className="text-gray-900 font-bold">:</span>
              <span className="rounded-md bg-gray-900 px-1.5 py-0.5">48</span>
            </div>
          </div>
          <Link
            href="/#flash-sale"
            className="text-xs font-medium text-primary-600 hover:underline"
          >
            Xem tất cả &gt;
          </Link>
        </div>

        {/* Flash Sale 6-Item Row */}
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-6 gap-3">
          {flashSaleItems.map((item) => {
            const imageSrc =
              item.imageKeys && item.imageKeys.length > 0
                ? getImageUrl(item.imageKeys[0])
                : item.imageUrl;

            return (
              <Link
                key={item.id}
                href={`/listing/${item.id}`}
                className="group flex flex-col items-center text-center p-2.5 rounded-xl hover:shadow-preline-hover hover:-translate-y-0.5 transition duration-200 bg-white border border-gray-100"
              >
                <div className="relative aspect-square w-full overflow-hidden bg-gray-50 rounded-lg">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={imageSrc}
                    alt={item.title}
                    className="h-full w-full object-cover group-hover:scale-105 transition duration-300"
                  />
                  <div className="absolute top-1.5 right-1.5">
                    <Badge variant="discount" size="xs">
                      -25%
                    </Badge>
                  </div>
                </div>

                <div className="mt-3 w-full">
                  <PriceTag
                    price={item.price}
                    size="sm"
                    className="justify-center"
                  />

                  {/* Flame progress bar */}
                  <div className="relative mt-2 h-3.5 w-full rounded-full bg-orange-100 overflow-hidden text-xs font-black text-white flex items-center justify-center">
                    <div
                      className="absolute left-0 top-0 h-full bg-gradient-to-r from-red-600 to-primary-500 rounded-full"
                      style={{ width: "82%" }}
                    />
                    <span className="relative z-10 text-xs uppercase tracking-wider drop-shadow-xs">
                      🔥 ĐÃ BÁN 82%
                    </span>
                  </div>
                </div>
              </Link>
            );
          })}
        </div>
      </Card>

      {/* ── 5. THƯƠNG HIỆU CHÍNH HÃNG (Official Brands) ── */}
      {mallItems.length > 0 && (
        <Card className="rounded-2xl border-gray-200/80 p-5 space-y-4">
          <div className="flex items-center justify-between border-b border-gray-100 pb-3">
            <div className="flex items-center gap-3">
              <span className="font-black text-sm sm:text-base text-red-600 uppercase tracking-wider flex items-center gap-1.5">
                <Badge variant="mall" size="sm">
                  MALL
                </Badge>
                <span>THƯƠNG HIỆU CHÍNH HÃNG</span>
              </span>
              <div className="hidden sm:flex items-center gap-4 text-xs text-gray-500 font-medium">
                <span>✓ 7 Ngày Miễn Phí Trả Hàng</span>
                <span>✓ Hàng Chính Hãng 100%</span>
                <span>✓ Miễn Phí Vận Chuyển</span>
              </div>
            </div>
            <Link
              href="/search?q=mall"
              className="text-xs font-medium text-red-600 hover:underline"
            >
              Xem tất cả &gt;
            </Link>
          </div>

          <ListingGrid
            listings={mallItems.slice(0, 6)}
            empty="Chưa có sản phẩm chính hãng nào."
          />
        </Card>
      )}

      {/* ── 6. GỢI Ý HÔM NAY (AI Recommendation Feed) ── */}
      <section className="space-y-4">
        {/* Sticky Tab Header */}
        <div className="rounded-2xl bg-white px-6 py-4 shadow-preline-card border border-gray-200/80 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="font-bold text-sm text-primary-600 uppercase tracking-wider flex items-center gap-2">
              <span className="p-1 rounded-lg bg-primary-50 text-base">🤖</span>
              <span>GỢI Ý HÔM NAY (AI RECOMMENDATION)</span>
            </span>
          </div>
          <span className="text-xs text-gray-500 font-normal">
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
          <Link href="/search">
            <Button
              variant="outline"
              size="lg"
              className="px-12 uppercase tracking-wider text-xs font-semibold text-gray-700 hover:border-primary-500 hover:text-primary-600 shadow-sm"
            >
              Xem Thêm Gợi Ý
            </Button>
          </Link>
        </div>
      </section>

      {/* ── "Vừa xem" — recently-viewed listings ── */}
      <RecentlyViewedRow />

      {/* ── 7. "Gợi ý cho bạn" — personalized recommendations ── */}
      <RecommendationsRow />

      {/* ── 8. Floating AI Assistant Component ── */}
      <AiAssistantModal listings={items} />
    </div>
  );
}
