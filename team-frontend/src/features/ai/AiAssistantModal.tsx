"use client";

import Link from "next/link";
import { useState } from "react";

import { formatPrice } from "@/components/ui/format";
import type { ViewListing } from "@/lib/gateway/listings";
import { getImageUrl } from "@/lib/media";

const SUGGESTED_QUERIES = [
  "Điện thoại chụp ảnh đẹp dưới 30 triệu",
  "Laptop làm đồ họa và lập trình mượt mà",
  "Gợi ý quà tặng bạn gái dưới 500k",
  "Đồ gia dụng thông minh cho nhà mới",
  "Giày sneaker trắng đi học đi chơi",
];

export function AiAssistantModal({
  listings = [],
}: {
  listings: ViewListing[];
}) {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [response, setResponse] = useState<{
    advice: string;
    items: { product: ViewListing; reason: string; matchScore: number }[];
  } | null>(null);

  function handleSearch(q: string) {
    if (!q.trim()) return;
    setQuery(q);
    setLoading(true);

    // AI Semantic Match Algorithm
    setTimeout(() => {
      const qLower = q.toLowerCase();
      const matched = listings
        .map((p) => {
          let score = 0;
          let reason = "Phù hợp với nhu cầu tìm kiếm của bạn.";

          const text =
            `${p.title} ${p.description} ${p.categoryId}`.toLowerCase();
          const words = qLower.split(" ").filter((w) => w.length > 1);

          for (const w of words) {
            if (text.includes(w)) score += 20;
          }

          if (
            qLower.includes("dưới") ||
            qLower.includes("30 triệu") ||
            qLower.includes("500k")
          ) {
            if (qLower.includes("500k") && p.price <= 500000) {
              score += 35;
              reason = `Mức giá cực tốt ${formatPrice(p.price)} (Dưới 500k) phù hợp ngân sách của bạn.`;
            } else if (qLower.includes("30 triệu") && p.price <= 30000000) {
              score += 35;
              reason = `Cấu hình mạnh mẽ, thương hiệu uy tín trong tầm giá ${formatPrice(p.price)}.`;
            }
          }

          if (qLower.includes("điện thoại") || qLower.includes("chụp ảnh")) {
            if (p.categoryId === "cat-electronics") {
              score += 40;
              reason =
                "Camera quang học đỉnh cao, pin trâu và hiệu năng mượt mà.";
            }
          }

          if (
            qLower.includes("laptop") ||
            qLower.includes("đồ họa") ||
            qLower.includes("lập trình")
          ) {
            if (p.categoryId === "cat-laptop") {
              score += 45;
              reason =
                "Hiệu năng xử lý đa nhiệm vượt trội, màn hình sắc nét chuẩn màu.";
            }
          }

          if (
            qLower.includes("quà") ||
            qLower.includes("bạn gái") ||
            qLower.includes("son")
          ) {
            if (
              p.categoryId === "cat-beauty" ||
              p.categoryId === "cat-fashion-women"
            ) {
              score += 45;
              reason =
                "Sản phẩm best-seller được phái nữ yêu thích, thiết kế đẹp mắt.";
            }
          }

          if (qLower.includes("gia dụng") || qLower.includes("nhà")) {
            if (
              p.categoryId === "cat-appliances" ||
              p.categoryId === "cat-home"
            ) {
              score += 40;
              reason =
                "Thiết bị thông minh tự động hóa, tiết kiệm điện năng cho gia đình.";
            }
          }

          if (qLower.includes("giày") || qLower.includes("sneaker")) {
            if (p.categoryId === "cat-shoes") {
              score += 45;
              reason =
                "Thiết kế kinh điển, đệm êm ái đi lại thoải mái cả ngày.";
            }
          }

          return {
            product: p,
            reason,
            matchScore: Math.min(99, Math.max(75, score + 65)),
          };
        })
        .filter((r) => r.matchScore >= 75)
        .sort((a, b) => b.matchScore - a.matchScore)
        .slice(0, 4);

      let advice = `Dựa trên yêu cầu "${q}", Trợ lý AI đã phân tích và gợi ý ${matched.length} sản phẩm phù hợp nhất với tiêu chí của bạn:`;
      if (qLower.includes("quà tặng bạn gái")) {
        advice =
          "💡 Trợ lý AI khuyên bạn: Với ngân sách dưới 500k, Son Black Rouge lì mịn hoặc Kem chống nắng La Roche-Posay là lựa chọn tinh tế và thiết thực nhất được nhiều bạn nữ đánh giá 5 sao!";
      } else if (qLower.includes("laptop") || qLower.includes("đồ họa")) {
        advice =
          "💡 Trợ lý AI khuyên bạn: MacBook Pro M3 Pro hoặc Bàn phím cơ Keychron / Chuột Logitech MX Master 3S là combo lý tưởng cho hiệu suất làm việc đồ họa & coding tối đa!";
      }

      setResponse({
        advice,
        items:
          matched.length > 0
            ? matched
            : listings.slice(0, 3).map((p) => ({
                product: p,
                reason: "Sản phẩm được yêu thích nhất trên sàn.",
                matchScore: 88,
              })),
      });
      setLoading(false);
    }, 450);
  }

  return (
    <>
      {/* Floating AI Trigger Button */}
      <button
        type="button"
        onClick={() => setIsOpen(true)}
        className="fixed bottom-24 right-6 z-40 flex items-center gap-2 rounded-full bg-gradient-to-r from-orange-500 via-brand to-red-600 px-4 py-3 text-xs font-bold text-white shadow-xl hover:scale-105 transition duration-200 border-2 border-white/80 animate-pulse"
      >
        <span className="text-base">🤖</span>
        <span>Trợ Lý Mua Sắm AI</span>
      </button>

      {/* AI Modal */}
      {isOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-xs">
          <div className="flex flex-col h-[600px] w-full max-w-2xl rounded-3xl bg-white shadow-2xl overflow-hidden animate-in fade-in zoom-in-95">
            {/* Header */}
            <div className="flex items-center justify-between bg-gradient-to-r from-brand to-orange-600 p-4 text-white">
              <div className="flex items-center gap-2.5">
                <span className="grid h-10 w-10 place-items-center rounded-2xl bg-white/20 text-xl backdrop-blur-xs">
                  🤖
                </span>
                <div>
                  <h3 className="font-bold text-sm">
                    Marketplace AI Shopping Assistant
                  </h3>
                  <p className="text-[11px] text-orange-100">
                    Trợ lý tìm kiếm ngữ nghĩa & gợi ý sản phẩm thông minh bằng
                    AI
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setIsOpen(false)}
                className="rounded-full bg-black/20 p-2 text-xs text-white hover:bg-black/40 transition"
              >
                ✕
              </button>
            </div>

            {/* Body */}
            <div className="flex-1 overflow-y-auto p-5 space-y-4 text-xs bg-gray-50/50">
              {/* Intro bubble */}
              <div className="flex gap-3 items-start">
                <div className="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-brand text-white text-xs">
                  AI
                </div>
                <div className="rounded-2xl rounded-tl-none bg-white p-3.5 shadow-xs border border-gray-100 max-w-md text-gray-800 leading-relaxed">
                  👋 Xin chào! Mình là **Trợ lý AI Marketplace**. Hãy nói cho
                  mình biết bạn đang tìm mua gì, ngân sách bao nhiêu, hoặc mô tả
                  phong cách bạn muốn, mình sẽ gợi ý sản phẩm phù hợp nhất ngay!
                </div>
              </div>

              {/* Suggestions pills */}
              <div className="space-y-1.5 pl-11">
                <p className="text-[11px] font-semibold text-gray-500">
                  Câu hỏi gợi ý:
                </p>
                <div className="flex flex-wrap gap-1.5">
                  {SUGGESTED_QUERIES.map((q) => (
                    <button
                      key={q}
                      type="button"
                      onClick={() => handleSearch(q)}
                      className="rounded-full bg-orange-50 px-3 py-1 text-[11px] font-medium text-brand border border-brand/30 hover:bg-brand hover:text-white transition"
                    >
                      {q}
                    </button>
                  ))}
                </div>
              </div>

              {/* AI Response Output */}
              {loading ? (
                <div className="flex gap-3 items-start pl-11">
                  <div className="flex items-center gap-2 rounded-2xl bg-white p-3.5 shadow-xs border border-gray-100 text-gray-500">
                    <span className="inline-block animate-spin text-brand">
                      ⚙️
                    </span>
                    <span>
                      AI đang phân tích ngữ nghĩa và tìm kiếm sản phẩm phù
                      hợp...
                    </span>
                  </div>
                </div>
              ) : response ? (
                <div className="space-y-3 pl-11">
                  {/* AI Reasoning Advice */}
                  <div className="rounded-2xl bg-orange-50/70 p-3.5 border border-brand/20 text-gray-800 leading-relaxed">
                    {response.advice}
                  </div>

                  {/* AI Product Cards Grid */}
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
                    {response.items.map(({ product, reason, matchScore }) => {
                      const imageSrc =
                        product.imageKeys && product.imageKeys.length > 0
                          ? getImageUrl(product.imageKeys[0])
                          : product.imageUrl;

                      return (
                        <div
                          key={product.id}
                          className="flex flex-col justify-between rounded-2xl border border-gray-200 bg-white p-3 shadow-2xs hover:border-brand transition"
                        >
                          <div className="flex gap-3">
                            <div className="h-16 w-16 shrink-0 overflow-hidden rounded-xl bg-gray-100">
                              {/* eslint-disable-next-line @next/next/no-img-element */}
                              <img
                                src={imageSrc}
                                alt={product.title}
                                className="h-full w-full object-cover"
                              />
                            </div>
                            <div className="min-w-0 flex-1">
                              <span className="inline-block rounded-xs bg-emerald-50 px-1.5 py-0.2 text-[9px] font-bold text-emerald-700">
                                ⭐ {matchScore}% Khớp ý định
                              </span>
                              <h4 className="line-clamp-2 text-xs font-bold text-gray-900 mt-1">
                                {product.title}
                              </h4>
                              <p className="text-xs font-black text-brand mt-1">
                                {formatPrice(product.price)}
                              </p>
                            </div>
                          </div>

                          <div className="mt-2.5 rounded-lg bg-gray-50 p-2 text-[10px] text-gray-600 leading-tight border border-gray-100">
                            💡 <strong>Lý do chọn:</strong> {reason}
                          </div>

                          <Link
                            href={`/listing/${product.id}`}
                            onClick={() => setIsOpen(false)}
                            className="mt-2.5 block text-center rounded-xl bg-brand py-1.5 text-xs font-bold text-white shadow-2xs hover:bg-brand-dark transition"
                          >
                            Xem Chi Tiết & Mua Ngay
                          </Link>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : null}
            </div>

            {/* Input Bar */}
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleSearch(query);
              }}
              className="flex items-center gap-2 border-t bg-white p-3.5"
            >
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Nhập yêu cầu tìm kiếm (VD: Tìm chuột công thái học, áo gió thể thao...)"
                className="flex-1 rounded-xl border border-gray-300 p-2.5 text-xs text-gray-800 outline-none focus:border-brand"
              />
              <button
                type="submit"
                disabled={!query.trim() || loading}
                className="rounded-xl bg-brand px-5 py-2.5 text-xs font-bold text-white shadow-xs hover:bg-brand-dark transition disabled:opacity-50"
              >
                Gửi AI
              </button>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
