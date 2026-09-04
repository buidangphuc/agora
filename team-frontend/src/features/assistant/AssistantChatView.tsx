"use client";

import Link from "next/link";
import React, { useState } from "react";

import { askAssistantAction } from "./actions";

interface ProductCard {
  listing_id: string;
  title: string;
  price: number;
  currency: string;
  image_url: string;
  discount_rate: number;
  rating_text: string;
}

interface ChatMessage {
  id: string;
  sender: "user" | "ai";
  text: string;
  product_cards?: ProductCard[];
  suggested_followups?: string[];
  time: string;
}

export function AssistantChatView() {
  const [messages, setMessages] = useState<ChatMessage[]>([
    {
      id: "msg_1",
      sender: "ai",
      text: "👋 Xin chào! Tôi là Trợ lý Mua sắm AI thông minh của sàn. Bạn đang tìm kiếm sản phẩm gì hôm nay? (Ví dụ: 'Tìm laptop dưới 25 triệu cho sinh viên IT', 'Gợi ý quà tặng bạn gái', 'Tai nghe chống ồn tốt nhất')",
      suggested_followups: [
        "iPhone 15 Pro Max có khuyến mãi gì?",
        "Tìm tai nghe chống ồn dưới 7 triệu",
        "Áo sơ mi nam công sở cao cấp",
      ],
      time: "Vừa xong",
    },
  ]);

  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSend = async (textToSend?: string) => {
    const query = textToSend || input;
    if (!query.trim()) return;

    const history = messages.map((m) => m.text);

    const userMsg: ChatMessage = {
      id: `user_${Date.now()}`,
      sender: "user",
      text: query,
      time: new Date().toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      }),
    };

    setMessages((prev) => [...prev, userMsg]);
    if (!textToSend) setInput("");
    setLoading(true);

    const now = new Date().toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
    });
    const aiId = `ai_${Date.now()}`;
    // Placeholder AI bubble we fill token-by-token as the stream arrives.
    setMessages((prev) => [
      ...prev,
      { id: aiId, sender: "ai", text: "", time: now },
    ]);

    const patchAi = (patch: Partial<ChatMessage>) =>
      setMessages((prev) =>
        prev.map((m) => (m.id === aiId ? { ...m, ...patch } : m)),
      );

    // 1) Stream reply text token-by-token (gateway → team-ai ChatService.StreamChat).
    let streamed = "";
    try {
      const res = await fetch("/api/assistant/stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: query }),
      });
      if (res.ok && res.body) {
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        for (;;) {
          const { value, done } = await reader.read();
          if (done) break;
          streamed += decoder.decode(value, { stream: true });
          patchAi({ text: streamed });
        }
      }
    } catch {
      // fall through to the unary call below
    }

    // 2) Unary call for the grounded reply + product cards + follow-ups.
    const result = await askAssistantAction(query, history);
    if (result.ok && result.reply) {
      const { reply } = result;
      patchAi({
        text: reply.replyText || streamed,
        product_cards: reply.productCards.map((c) => ({
          listing_id: c.listingId,
          title: c.title,
          price: c.price,
          currency: c.currency,
          image_url: c.imageUrl,
          discount_rate: c.discountRate,
          rating_text: c.ratingText,
        })),
        suggested_followups: reply.suggestedFollowups,
      });
    } else if (!streamed) {
      patchAi({
        text:
          result.message ||
          "Xin lỗi, trợ lý AI đang tạm thời không phản hồi. Vui lòng thử lại.",
      });
    }
    setLoading(false);
  };

  return (
    <div className="max-w-4xl mx-auto p-4 sm:p-6 min-h-[calc(100vh-140px)] flex flex-col">
      {/* Header */}
      <div className="bg-gradient-to-r from-purple-600 via-indigo-600 to-blue-600 rounded-2xl p-6 text-white shadow-lg mb-4 flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <span className="px-2.5 py-0.5 rounded-full text-xs font-bold bg-white/20 backdrop-blur-sm">
              ✨ RAG AI COPILOT
            </span>
            <h1 className="text-xl sm:text-2xl font-black tracking-tight">
              Trợ Lý Mua Sắm AI Thông Minh
            </h1>
          </div>
          <p className="text-sm text-purple-100 mt-1">
            Hiểu ngôn ngữ tự nhiên, gợi ý sản phẩm tức thì kèm thẻ tương tác
            1-click
          </p>
        </div>
        <div className="hidden sm:block text-4xl">🤖</div>
      </div>

      {/* Messages Container */}
      <div className="flex-1 bg-white rounded-2xl border border-slate-200 shadow-sm p-4 sm:p-6 overflow-y-auto space-y-5">
        {messages.map((msg) => (
          <div
            key={msg.id}
            className={`flex flex-col ${msg.sender === "user" ? "items-end" : "items-start"}`}
          >
            <div className="flex items-start gap-2.5 max-w-[85%]">
              {msg.sender === "ai" && (
                <div className="w-8 h-8 rounded-full bg-indigo-600 text-white flex items-center justify-center font-bold text-xs shrink-0 shadow">
                  AI
                </div>
              )}

              <div
                className={`p-4 rounded-2xl text-sm leading-relaxed ${
                  msg.sender === "user"
                    ? "bg-indigo-600 text-white rounded-tr-none shadow-md"
                    : "bg-slate-50 text-slate-800 border border-slate-200 rounded-tl-none"
                }`}
              >
                <p className="whitespace-pre-wrap">{msg.text}</p>

                {/* Embedded Product Cards */}
                {msg.product_cards && msg.product_cards.length > 0 && (
                  <div className="mt-3.5 space-y-3">
                    {msg.product_cards.map((p) => (
                      <div
                        key={p.listing_id}
                        className="bg-white border border-slate-200 rounded-xl p-3 shadow-sm hover:shadow-md transition-all flex flex-col sm:flex-row items-center gap-3.5"
                      >
                        <img
                          src={p.image_url}
                          alt={p.title}
                          className="w-20 h-20 object-cover rounded-lg shrink-0"
                        />
                        <div className="flex-1 min-w-0">
                          <h4 className="font-bold text-slate-900 text-sm line-clamp-1">
                            {p.title}
                          </h4>
                          <div className="text-xs text-amber-600 font-medium mt-0.5">
                            {p.rating_text}
                          </div>
                          <div className="flex items-center gap-2 mt-1">
                            <span className="text-base font-black text-red-600">
                              {p.price.toLocaleString()} ₫
                            </span>
                            {p.discount_rate > 0 && (
                              <span className="text-[10px] font-bold bg-red-100 text-red-700 px-1.5 py-0.5 rounded">
                                -{p.discount_rate}%
                              </span>
                            )}
                          </div>
                        </div>

                        <div className="flex sm:flex-col gap-2 w-full sm:w-auto">
                          <Link
                            href={`/listing/${p.listing_id}`}
                            className="flex-1 sm:flex-none px-3.5 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-semibold rounded-lg text-center transition-all shadow-sm"
                          >
                            Xem Ngay
                          </Link>
                          <Link
                            href={"/checkout"}
                            className="flex-1 sm:flex-none px-3.5 py-1.5 bg-red-600 hover:bg-red-700 text-white text-xs font-semibold rounded-lg text-center transition-all shadow-sm"
                          >
                            Mua Ngay
                          </Link>
                        </div>
                      </div>
                    ))}
                  </div>
                )}

                <div className="text-[10px] text-slate-400 mt-2 text-right">
                  {msg.time}
                </div>
              </div>
            </div>

            {/* Suggested Followups */}
            {msg.suggested_followups && msg.suggested_followups.length > 0 && (
              <div className="flex flex-wrap gap-2 mt-2 ml-10">
                {msg.suggested_followups.map((f) => (
                  <button
                    key={f}
                    type="button"
                    onClick={() => handleSend(f)}
                    className="px-3 py-1 bg-indigo-50 hover:bg-indigo-100 text-indigo-700 border border-indigo-200 rounded-full text-xs font-medium transition-all shadow-2xs text-left"
                  >
                    💡 {f}
                  </button>
                ))}
              </div>
            )}
          </div>
        ))}

        {loading && (
          <div className="flex items-center gap-2 text-slate-400 text-xs italic pl-10">
            <div className="w-2 h-2 bg-indigo-600 rounded-full animate-bounce" />
            <div className="w-2 h-2 bg-indigo-600 rounded-full animate-bounce [animation-delay:0.2s]" />
            <div className="w-2 h-2 bg-indigo-600 rounded-full animate-bounce [animation-delay:0.4s]" />
            <span>AI đang truy vấn catalog và phân tích vector...</span>
          </div>
        )}
      </div>

      {/* Input Box */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSend();
        }}
        className="mt-4 flex gap-2"
      >
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Nhập câu hỏi mua sắm (VD: 'Tìm tai nghe pin trâu dưới 5 triệu')..."
          className="flex-1 px-4 py-3 bg-white border border-slate-300 rounded-xl text-sm focus:outline-hidden focus:ring-2 focus:ring-indigo-500 shadow-sm"
        />
        <button
          type="submit"
          disabled={loading || !input.trim()}
          className="px-6 py-3 bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white font-bold text-sm rounded-xl transition-all shadow-md flex items-center gap-2"
        >
          <span>Gửi</span>
          <span>➔</span>
        </button>
      </form>
    </div>
  );
}
