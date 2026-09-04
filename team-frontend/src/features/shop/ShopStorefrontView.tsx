"use client";

import Link from "next/link";
import { useState } from "react";

import { ChatWithSellerButton } from "@/features/chat/ChatWithSellerButton";
import { FollowSellerButton } from "@/features/engagement/FollowSellerButton";
import { ListingGrid } from "@/features/listing/ListingGrid";
import type { ViewListing, ViewStorefront } from "@/lib/gateway/listings";

export function ShopStorefrontView({
  sellerId,
  listings,
  loggedIn,
  initialFollowing = false,
  storefront = null,
}: {
  sellerId: string;
  listings: ViewListing[];
  loggedIn: boolean;
  initialFollowing?: boolean;
  storefront?: ViewStorefront | null;
}) {
  const [tab, setTab] = useState<string>("all");

  const sortedListings =
    tab === "price_asc"
      ? [...listings].sort((a, b) => a.price - b.price)
      : tab === "price_desc"
        ? [...listings].sort((a, b) => b.price - a.price)
        : listings;

  return (
    <div className="space-y-6">
      {/* ── Storefront banner + tagline (team-listing StorefrontService) ── */}
      {storefront && (storefront.bannerUrl || storefront.tagline) && (
        <div className="overflow-hidden rounded-2xl border border-gray-100 bg-white shadow-xs">
          {storefront.bannerUrl && (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={storefront.bannerUrl}
              alt="Ảnh bìa gian hàng"
              className="h-32 w-full object-cover sm:h-44"
            />
          )}
          {storefront.tagline && (
            <p className="px-5 py-3 text-sm font-medium text-gray-700">
              {storefront.tagline}
            </p>
          )}
        </div>
      )}

      {/* ── Shop Profile Header ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-6 shadow-xs">
        <div className="grid grid-cols-1 gap-6 md:grid-cols-12 md:items-center">
          {/* Shop Card */}
          <div className="md:col-span-5 flex items-center gap-4 rounded-xl bg-gradient-to-br from-orange-600 to-red-500 p-5 text-white shadow-sm">
            <div className="grid h-16 w-16 place-items-center rounded-full bg-white/20 text-3xl font-black border-2 border-white/40">
              🏪
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <h1 className="text-base font-bold truncate">
                  Shop Chính Hãng #{sellerId.slice(0, 6)}
                </h1>
                <span className="rounded bg-white text-red-600 px-1.5 py-0.2 text-[9px] font-extrabold">
                  Mall
                </span>
              </div>
              <p className="text-xs text-orange-100 mt-0.5">
                Online 5 phút trước · Toàn quốc
              </p>

              <div className="mt-3 flex items-center gap-2">
                <FollowSellerButton
                  sellerId={sellerId}
                  initialFollowing={initialFollowing}
                />
                <ChatWithSellerButton
                  sellerId={sellerId}
                  listingId={listings[0]?.id ?? ""}
                  loggedIn={loggedIn}
                />
              </div>
            </div>
          </div>

          {/* Shop Stats */}
          <div className="md:col-span-7 grid grid-cols-2 sm:grid-cols-4 gap-4 text-xs text-gray-600 pl-0 md:pl-6">
            <div className="rounded-lg bg-gray-50/70 p-3">
              <p className="text-gray-400">Sản Phẩm</p>
              <p className="font-bold text-sm text-gray-900 mt-0.5">
                {listings.length}
              </p>
            </div>
            <div className="rounded-lg bg-gray-50/70 p-3">
              <p className="text-gray-400">Đánh Giá</p>
              <p className="font-bold text-sm text-brand mt-0.5">
                4.9 / 5.0 ⭐
              </p>
            </div>
            <div className="rounded-lg bg-gray-50/70 p-3">
              <p className="text-gray-400">Tỉ Lệ Phản Hồi</p>
              <p className="font-bold text-sm text-emerald-600 mt-0.5">99%</p>
            </div>
            <div className="rounded-lg bg-gray-50/70 p-3">
              <p className="text-gray-400">Tham Gia</p>
              <p className="font-bold text-sm text-gray-900 mt-0.5">1 năm</p>
            </div>
          </div>
        </div>
      </div>

      {/* ── Shop Storefront Tabs ── */}
      <div className="flex border-b border-gray-200 bg-white p-2 rounded-xl shadow-2xs text-xs font-semibold text-gray-600 gap-2">
        <button
          type="button"
          onClick={() => setTab("all")}
          className={`py-2 px-4 rounded-lg transition ${
            tab === "all"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          Tất cả sản phẩm ({listings.length})
        </button>
        <button
          type="button"
          onClick={() => setTab("price_asc")}
          className={`py-2 px-4 rounded-lg transition ${
            tab === "price_asc"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          Giá: Thấp đến Cao
        </button>
        <button
          type="button"
          onClick={() => setTab("price_desc")}
          className={`py-2 px-4 rounded-lg transition ${
            tab === "price_desc"
              ? "bg-brand text-white font-bold shadow-2xs"
              : "hover:bg-gray-100"
          }`}
        >
          Giá: Cao đến Thấp
        </button>
      </div>

      {/* ── Products Grid ── */}
      <div className="space-y-4">
        <h2 className="text-sm font-bold uppercase tracking-wider text-gray-800">
          SẢN PHẨM CỦA SHOP ({sortedListings.length})
        </h2>
        <ListingGrid
          listings={sortedListings}
          empty="Shop này hiện chưa có sản phẩm nào đang bày bán."
        />
      </div>
    </div>
  );
}
