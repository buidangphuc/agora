"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import type { ViewListing } from "@/lib/gateway/listings";
import { getImageUrl } from "@/lib/media";

export function FlashSaleSection({ listings }: { listings: ViewListing[] }) {
  const [timeLeft, setTimeLeft] = useState({
    hours: 2,
    minutes: 45,
    seconds: 30,
  });

  useEffect(() => {
    const timer = setInterval(() => {
      setTimeLeft((prev) => {
        if (prev.seconds > 0) {
          return { ...prev, seconds: prev.seconds - 1 };
        }
        if (prev.minutes > 0) {
          return { ...prev, minutes: prev.minutes - 1, seconds: 59 };
        }
        if (prev.hours > 0) {
          return { hours: prev.hours - 1, minutes: 59, seconds: 59 };
        }
        return { hours: 3, minutes: 0, seconds: 0 };
      });
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  if (listings.length === 0) return null;

  return (
    <div className="rounded-xl border border-orange-100 bg-white p-4 shadow-xs">
      {/* ── Header ── */}
      <div className="flex flex-wrap items-center justify-between border-b pb-3.5">
        <div className="flex items-center gap-3">
          <span className="text-xl font-extrabold tracking-wide text-brand flex items-center gap-1">
            <span>⚡</span>
            <span>FLASH SALE</span>
          </span>
          <div className="flex items-center gap-1 text-xs font-bold text-white">
            <span className="rounded bg-gray-900 px-1.5 py-0.5">
              {String(timeLeft.hours).padStart(2, "0")}
            </span>
            <span className="text-gray-800">:</span>
            <span className="rounded bg-gray-900 px-1.5 py-0.5">
              {String(timeLeft.minutes).padStart(2, "0")}
            </span>
            <span className="text-gray-800">:</span>
            <span className="rounded bg-gray-900 px-1.5 py-0.5">
              {String(timeLeft.seconds).padStart(2, "0")}
            </span>
          </div>
        </div>

        <Link
          href="/search"
          className="text-xs font-semibold text-brand hover:underline"
        >
          Xem tất cả &gt;
        </Link>
      </div>

      {/* ── Deals Grid ── */}
      <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {listings.slice(0, 6).map((l, i) => {
          const discount = 25 + ((i * 7) % 35);
          const soldProgress = 40 + ((i * 12) % 55);
          const imageSrc =
            l.imageKeys && l.imageKeys.length > 0
              ? getImageUrl(l.imageKeys[0])
              : l.imageUrl;

          return (
            <Link
              key={l.id}
              href={`/listing/${l.id}`}
              className="group flex flex-col overflow-hidden rounded-md border border-gray-100 p-2 transition hover:-translate-y-0.5 hover:border-brand hover:shadow-sm"
            >
              <div className="relative aspect-square w-full overflow-hidden rounded bg-gray-50">
                {imageSrc ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={imageSrc}
                    alt={l.title}
                    className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
                  />
                ) : (
                  <div className="flex h-full w-full items-center justify-center text-2xl text-gray-300">
                    ⚡
                  </div>
                )}
                <div className="absolute top-0 right-0 rounded-bl bg-yellow-400 px-1 text-[9px] font-extrabold text-red-600">
                  -{discount}%
                </div>
              </div>

              <div className="mt-2 text-center">
                <div className="text-sm font-bold text-brand">
                  ₫{l.price.toLocaleString("vi-VN")}
                </div>

                {/* Progress bar */}
                <div className="relative mt-1.5 h-3.5 w-full overflow-hidden rounded-full bg-red-100">
                  <div
                    className="h-full rounded-full bg-gradient-to-r from-orange-500 to-red-500"
                    style={{ width: `${soldProgress}%` }}
                  />
                  <span className="absolute inset-0 flex items-center justify-center text-[9px] font-bold uppercase text-white">
                    🔥 ĐÃ BÁN {soldProgress}
                  </span>
                </div>
              </div>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
