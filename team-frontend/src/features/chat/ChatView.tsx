"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { MessageType } from "@/generated/platform/chat/v1/chat_pb.js";
import type { ViewChatMessage, ViewChatThread } from "@/lib/gateway/chat";
import {
  chatCopilotAction,
  fetchThreadMessagesAction,
  listQuickRepliesAction,
  markThreadReadAction,
  searchMessagesAction,
  sendMessageAction,
} from "./actions";

// Default fallback suggestions before the seller asks the AI copilot.
const DEFAULT_COPILOT_REPLIES = [
  "Dạ shop còn sẵn hàng giao ngay trong 2h ạ! 🚀",
  "Đơn hàng của bạn đang được đóng gói và giao sớm nhé! 📦",
  "Sản phẩm được bảo hành chính hãng 12 tháng ạ! ⭐",
];

export function ChatView({
  threads: initialThreads,
  activeThread: propActiveThread,
  initialMessages,
  currentUserId,
}: {
  threads: ViewChatThread[];
  activeThread?: ViewChatThread;
  initialMessages: ViewChatMessage[];
  currentUserId: string;
}) {
  const [threads, setThreads] = useState<ViewChatThread[]>(initialThreads);
  const [activeThread, setActiveThread] = useState<ViewChatThread | undefined>(
    propActiveThread ||
      (initialThreads.length > 0 ? initialThreads[0] : undefined),
  );
  const [messages, setMessages] = useState<ViewChatMessage[]>(initialMessages);
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [searchFilter, setSearchFilter] = useState("");
  const [searchResults, setSearchResults] = useState<ViewChatMessage[]>([]);
  const [searching, setSearching] = useState(false);
  const [copilotReplies, setCopilotReplies] = useState<string[]>(
    DEFAULT_COPILOT_REPLIES,
  );
  const [copilotLoading, setCopilotLoading] = useState(false);
  const [quickReplies, setQuickReplies] = useState<string[]>([]);
  const messagesContainerRef = useRef<HTMLDivElement>(null);

  const isSeller = !!activeThread && activeThread.sellerId === currentUserId;

  // Ask team-ai (through the gateway) for reply suggestions based on the last
  // buyer message in the active thread.
  async function loadCopilot() {
    if (!activeThread) return;
    const lastBuyerMsg = [...messages]
      .reverse()
      .find((m) => m.senderId !== currentUserId);
    setCopilotLoading(true);
    const res = await chatCopilotAction(
      activeThread.sellerId,
      lastBuyerMsg?.content ?? activeThread.lastMessageText ?? "",
      activeThread.listingId,
    );
    setCopilotLoading(false);
    if (res.ok && res.replies.length > 0) setCopilotReplies(res.replies);
  }

  const activeThreadId = activeThread?.id;

  // Mark read once when active thread ID changes
  useEffect(() => {
    if (activeThreadId) {
      markThreadReadAction(activeThreadId);
    }
  }, [activeThreadId]);

  // Buyer-facing quick-reply chips configured by the seller (team-chat via the
  // gateway). Loaded per active thread; empty when none / seller side.
  const activeSellerId = activeThread?.sellerId;
  useEffect(() => {
    if (!activeSellerId || isSeller) {
      setQuickReplies([]);
      return;
    }
    let cancelled = false;
    listQuickRepliesAction(activeSellerId).then((chips) => {
      if (!cancelled) setQuickReplies(chips);
    });
    return () => {
      cancelled = true;
    };
  }, [activeSellerId, isSeller]);

  // Sync initial messages when props change
  useEffect(() => {
    if (initialMessages.length > 0) {
      setMessages(initialMessages);
    }
  }, [initialMessages]);

  // Smooth scroll to bottom when messages update
  // biome-ignore lint/correctness/useExhaustiveDependencies: scroll when message list updates
  useEffect(() => {
    if (messagesContainerRef.current) {
      messagesContainerRef.current.scrollTop =
        messagesContainerRef.current.scrollHeight;
    }
  }, [messages.length]);

  // Periodic refresh messages for current active thread
  useEffect(() => {
    if (!activeThreadId) return;
    const interval = setInterval(async () => {
      try {
        const msgs = await fetchThreadMessagesAction(activeThreadId);
        if (msgs.length > 0) {
          setMessages((prev) => {
            if (msgs.length !== prev.length) return msgs;
            const lastNew = msgs[msgs.length - 1];
            const lastOld = prev[prev.length - 1];
            if (
              lastNew?.id !== lastOld?.id ||
              lastNew?.content !== lastOld?.content
            ) {
              return msgs;
            }
            return prev;
          });
        }
      } catch {
        // ignore polling error
      }
    }, 4000);
    return () => clearInterval(interval);
  }, [activeThreadId]);

  // Real message search through the gateway (team-chat SearchMessages),
  // debounced. Empty query clears results and restores the thread list.
  useEffect(() => {
    const q = searchFilter.trim();
    if (!q) {
      setSearchResults([]);
      setSearching(false);
      return;
    }
    setSearching(true);
    const handle = setTimeout(async () => {
      try {
        const msgs = await searchMessagesAction(q);
        setSearchResults(msgs);
      } catch {
        setSearchResults([]);
      } finally {
        setSearching(false);
      }
    }, 300);
    return () => clearTimeout(handle);
  }, [searchFilter]);

  // Jump to the thread a matched message belongs to (when it's in the list).
  function handleSelectSearchResult(m: ViewChatMessage) {
    const thread = threads.find((t) => t.id === m.threadId);
    if (thread) {
      handleSelectThread(thread);
    }
    setSearchFilter("");
  }

  async function handleSelectThread(t: ViewChatThread) {
    setActiveThread(t);
    // Keep URL clean as /chat
    if (typeof window !== "undefined") {
      window.history.replaceState(null, "", "/chat");
    }
    try {
      const msgs = await fetchThreadMessagesAction(t.id);
      setMessages(msgs);
    } catch {
      setMessages([]);
    }
  }

  async function handleSend(e: React.FormEvent) {
    e.preventDefault();
    if (!activeThread || !text.trim()) return;
    const content = text.trim();
    setText("");
    setSending(true);

    const tempMsg: ViewChatMessage = {
      id: `temp-${Date.now()}`,
      threadId: activeThread.id,
      senderId: currentUserId,
      senderName: "Tôi",
      content,
      createdAt: new Date().toLocaleTimeString("vi-VN", {
        hour: "2-digit",
        minute: "2-digit",
      }),
      messageType: MessageType.TEXT,
      listingId: "",
      payload: "",
    };
    setMessages((prev) => [...prev, tempMsg]);

    try {
      const res = await sendMessageAction(
        activeThread.id,
        content,
        activeThread.sellerId,
      );
      if (res.ok && res.newThreadId && res.newThreadId !== activeThread.id) {
        const updatedThread = { ...activeThread, id: res.newThreadId };
        setActiveThread(updatedThread);
        setThreads((prev) => [
          updatedThread,
          ...prev.filter((item) => item.id !== activeThread.id),
        ]);
      }
    } finally {
      setSending(false);
    }
  }

  const isSearching = searchFilter.trim().length > 0;

  return (
    <div className="flex h-[calc(100vh-280px)] min-h-[520px] max-h-[680px] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
      {/* Threads List (Left) */}
      <div
        className={`${
          activeThread ? "hidden sm:flex" : "flex w-full"
        } border-r border-gray-200 sm:w-80 md:w-96 flex-col bg-gray-50/40`}
      >
        <div className="border-b border-gray-200 p-4 bg-white">
          <div className="flex items-center justify-between mb-2">
            <h1 className="text-base font-bold text-gray-900 flex items-center gap-2">
              <span className="text-lg">💬</span> Hộp thư tin nhắn
            </h1>
            <span className="text-[11px] font-semibold text-brand bg-orange-50 px-2 py-0.5 rounded-full border border-orange-100">
              {threads.length} hội thoại
            </span>
          </div>
          <div className="relative">
            <input
              type="text"
              value={searchFilter}
              onChange={(e) => setSearchFilter(e.target.value)}
              placeholder="Tìm kiếm cuộc trò chuyện..."
              className="w-full rounded-xl border border-gray-200 bg-gray-50/80 px-3.5 py-1.5 text-xs text-gray-800 placeholder-gray-400 focus:bg-white focus:border-brand focus:outline-hidden transition"
            />
          </div>
        </div>

        <div className="flex-1 overflow-y-auto divide-y divide-gray-100">
          {isSearching ? (
            searching ? (
              <div className="p-8 text-center text-xs text-gray-400 flex flex-col items-center justify-center gap-2">
                <span className="text-2xl">🔎</span>
                <span>Đang tìm tin nhắn...</span>
              </div>
            ) : searchResults.length === 0 ? (
              <div className="p-8 text-center text-xs text-gray-400 flex flex-col items-center justify-center gap-2">
                <span className="text-2xl">📭</span>
                <span>Không tìm thấy tin nhắn nào.</span>
              </div>
            ) : (
              searchResults.map((m) => (
                <button
                  type="button"
                  key={m.id}
                  onClick={() => handleSelectSearchResult(m)}
                  className="w-full text-left flex items-start gap-3 p-3.5 transition cursor-pointer hover:bg-white/80"
                >
                  <div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-gradient-to-br from-brand/20 to-orange-100 text-sm font-bold text-brand shadow-2xs">
                    💬
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between mb-0.5">
                      <p className="truncate text-xs font-bold text-gray-900">
                        {m.senderName}
                      </p>
                      {m.createdAt && (
                        <span className="text-[10px] text-gray-400 font-normal">
                          {m.createdAt}
                        </span>
                      )}
                    </div>
                    <p className="truncate text-xs text-gray-500">
                      {m.content}
                    </p>
                  </div>
                </button>
              ))
            )
          ) : threads.length === 0 ? (
            <div className="p-8 text-center text-xs text-gray-400 flex flex-col items-center justify-center gap-2">
              <span className="text-2xl">📭</span>
              <span>Không tìm thấy cuộc trò chuyện nào.</span>
            </div>
          ) : (
            threads.map((t) => {
              const isSelected = t.id === activeThread?.id;
              const isBuyer = t.buyerId === currentUserId;
              const unreadCount = isBuyer
                ? t.unreadCountBuyer
                : t.unreadCountSeller;
              const partnerRole = isBuyer ? "Người bán" : "Người mua";
              const partnerId = isBuyer ? t.sellerId : t.buyerId;

              return (
                <button
                  type="button"
                  key={t.id}
                  onClick={() => handleSelectThread(t)}
                  className={`w-full text-left flex items-start gap-3 p-3.5 transition cursor-pointer ${
                    isSelected
                      ? "bg-orange-50/80 border-l-4 border-brand font-medium shadow-2xs"
                      : "hover:bg-white/80"
                  }`}
                >
                  <div className="relative">
                    <div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-gradient-to-br from-brand/20 to-orange-100 text-sm font-bold text-brand shadow-2xs">
                      {isBuyer ? "🏪" : "👤"}
                    </div>
                    <span className="absolute bottom-0 right-0 h-2.5 w-2.5 rounded-full bg-emerald-500 ring-2 ring-white" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between mb-0.5">
                      <p className="truncate text-xs font-bold text-gray-900">
                        {partnerRole} #{partnerId.slice(0, 6)}
                      </p>
                      {t.lastMessageAt && (
                        <span className="text-[10px] text-gray-400 font-normal">
                          {t.lastMessageAt}
                        </span>
                      )}
                    </div>
                    {t.listingTitle && (
                      <p className="truncate text-[11px] text-brand font-medium mb-0.5">
                        📦 {t.listingTitle}
                      </p>
                    )}
                    <p className="truncate text-xs text-gray-500">
                      {t.lastMessageText || "Bắt đầu cuộc trò chuyện..."}
                    </p>
                  </div>
                  {unreadCount > 0 && (
                    <span className="rounded-full bg-brand px-1.5 py-0.5 text-[10px] font-bold text-white shadow-2xs">
                      {unreadCount}
                    </span>
                  )}
                </button>
              );
            })
          )}
        </div>
      </div>

      {/* Messages Panel (Right) */}
      <div
        className={`${
          activeThread ? "flex" : "hidden sm:flex"
        } flex-1 flex-col bg-white`}
      >
        {activeThread ? (
          <>
            {/* Header */}
            <div className="flex items-center justify-between border-b border-gray-200 px-4 sm:px-6 py-3.5 bg-white shadow-2xs">
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => setActiveThread(undefined)}
                  className="sm:hidden flex items-center justify-center h-8 w-8 rounded-lg bg-gray-50 border border-gray-200 text-gray-700 hover:text-brand transition cursor-pointer"
                  aria-label="Quay lại danh sách trò chuyện"
                >
                  ←
                </button>
                <div className="relative">
                  <div className="grid h-10 w-10 place-items-center rounded-full bg-gradient-to-br from-brand/10 to-orange-50 text-base font-bold text-brand shadow-2xs">
                    {activeThread.buyerId === currentUserId ? "🏪" : "👤"}
                  </div>
                  <span className="absolute bottom-0 right-0 h-2.5 w-2.5 rounded-full bg-emerald-500 ring-2 ring-white" />
                </div>
                <div>
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-sm text-gray-900">
                      {activeThread.buyerId === currentUserId
                        ? "Người bán"
                        : "Người mua"}{" "}
                      (
                      {activeThread.buyerId === currentUserId
                        ? `#${activeThread.sellerId.slice(0, 6)}`
                        : `#${activeThread.buyerId.slice(0, 6)}`}
                      )
                    </span>
                    <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2 py-0.5 text-[10px] font-semibold text-emerald-700 border border-emerald-200">
                      <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                      Trực tuyến
                    </span>
                  </div>
                  <div className="flex items-center gap-2 mt-0.5">
                    {activeThread.listingId ? (
                      <Link
                        href={`/listing/${activeThread.listingId}`}
                        className="text-xs text-brand hover:underline font-medium"
                      >
                        📦{" "}
                        {activeThread.listingTitle ||
                          `Xem sản phẩm #${activeThread.listingId.slice(0, 8)}`}
                      </Link>
                    ) : (
                      <span className="text-[11px] text-gray-400">
                        Cuộc trò chuyện trực tiếp 1:1
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {activeThread.buyerId === currentUserId && (
                <Link
                  href={`/shop/${activeThread.sellerId}`}
                  className="hidden md:inline-flex items-center gap-1 text-xs font-semibold text-gray-600 hover:text-brand border border-gray-200 rounded-lg px-3 py-1.5 transition hover:bg-gray-50 shadow-2xs"
                >
                  🏪 Xem gian hàng
                </Link>
              )}
            </div>

            {/* Message Stream */}
            <div
              ref={messagesContainerRef}
              className="flex-1 space-y-3.5 overflow-y-auto p-4 sm:p-6 bg-slate-50/50"
            >
              {messages.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-center p-8 max-w-sm mx-auto">
                  <div className="w-14 h-14 rounded-2xl bg-orange-100/80 text-brand grid place-items-center text-2xl mb-3 shadow-xs">
                    👋
                  </div>
                  <h3 className="text-sm font-bold text-gray-800 mb-1">
                    Bắt đầu cuộc trò chuyện
                  </h3>
                  <p className="text-xs text-gray-500 leading-relaxed">
                    Hãy gửi lời chào hoặc chọn các câu hỏi mẫu bên dưới để trao
                    đổi với đối tác về sản phẩm, giao hàng và khuyến mãi.
                  </p>
                </div>
              ) : (
                messages.map((m) => {
                  const isMe = m.senderId === currentUserId;
                  const isListingCard =
                    m.messageType === MessageType.LISTING_CARD;
                  return (
                    <div
                      key={m.id}
                      className={`flex flex-col ${
                        isMe ? "items-end" : "items-start"
                      }`}
                    >
                      {isListingCard ? (
                        <Link
                          href={`/listing/${m.listingId}`}
                          className="max-w-[75%] rounded-2xl border border-brand/30 bg-white p-3 text-xs shadow-2xs transition hover:border-brand"
                        >
                          <span className="inline-flex items-center gap-1 rounded-full bg-orange-50 px-2 py-0.5 text-[10px] font-bold text-brand">
                            📦 Sản phẩm
                          </span>
                          <p className="mt-1.5 font-semibold text-gray-900">
                            {m.content || `Sản phẩm #${m.listingId.slice(0, 8)}`}
                          </p>
                          {m.payload && (
                            <p className="mt-0.5 text-[11px] text-gray-500">
                              {m.payload}
                            </p>
                          )}
                          <span className="mt-1.5 inline-block text-[11px] font-semibold text-brand">
                            Xem chi tiết →
                          </span>
                        </Link>
                      ) : (
                        <div
                          className={`max-w-[75%] rounded-2xl px-4 py-2.5 text-xs shadow-2xs leading-relaxed ${
                            isMe
                              ? "bg-brand text-white rounded-tr-xs"
                              : "bg-white text-gray-800 border border-gray-200/80 rounded-tl-xs"
                          }`}
                        >
                          <p className="whitespace-pre-wrap font-normal">
                            {m.content}
                          </p>
                        </div>
                      )}
                      <span className="mt-1 text-[10px] text-gray-400 px-1 font-medium">
                        {m.createdAt}
                      </span>
                    </div>
                  );
                })
              )}
            </div>

            {/* ✨ AI Seller Copilot Quick Replies (from team-ai via the gateway) */}
            {isSeller && (
              <div className="px-4 py-2 bg-gradient-to-r from-indigo-50/90 to-blue-50/60 border-t border-indigo-100 flex items-center gap-2 overflow-x-auto text-[11px] [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]">
                <button
                  type="button"
                  onClick={loadCopilot}
                  disabled={copilotLoading}
                  className="text-indigo-900 font-bold shrink-0 flex items-center gap-1 hover:text-indigo-700 disabled:opacity-50 cursor-pointer"
                  title="Gợi ý trả lời bằng AI"
                >
                  <span>
                    {copilotLoading ? "✨ Đang gợi ý..." : "✨ Gợi ý AI"}
                  </span>
                  <span className="text-indigo-400">🔄</span>
                </button>
                {copilotReplies.map((sug) => (
                  <button
                    key={sug}
                    type="button"
                    onClick={() => setText(sug)}
                    className="px-3 py-1 bg-white hover:bg-indigo-100/80 text-indigo-700 border border-indigo-200/90 rounded-full shrink-0 font-medium transition shadow-2xs hover:border-indigo-300 cursor-pointer"
                  >
                    {sug}
                  </button>
                ))}
              </div>
            )}

            {/* 💬 Buyer quick-reply chips (seller-configured, team-chat) */}
            {!isSeller && quickReplies.length > 0 && (
              <div className="px-4 py-2 bg-gray-50 border-t border-gray-100 flex items-center gap-2 overflow-x-auto text-[11px] [&::-webkit-scrollbar]:hidden [-ms-overflow-style:none] [scrollbar-width:none]">
                <span className="shrink-0 font-bold text-gray-500">
                  Trả lời nhanh:
                </span>
                {quickReplies.map((qr) => (
                  <button
                    key={qr}
                    type="button"
                    onClick={() => setText(qr)}
                    className="px-3 py-1 bg-white hover:bg-orange-50 text-gray-700 border border-gray-200 rounded-full shrink-0 font-medium transition hover:border-brand cursor-pointer"
                  >
                    {qr}
                  </button>
                ))}
              </div>
            )}

            {/* Input Bar */}
            <form
              onSubmit={handleSend}
              className="flex items-center gap-2 border-t border-gray-200 p-3 sm:p-4 bg-white"
            >
              <div className="flex-1 relative flex items-center">
                <input
                  type="text"
                  value={text}
                  onChange={(e) => setText(e.target.value)}
                  placeholder="Nhập nội dung tin nhắn trao đổi..."
                  className="w-full rounded-xl border border-gray-300 bg-gray-50/50 px-4 py-2.5 text-xs text-gray-900 placeholder-gray-400 focus:bg-white focus:border-brand focus:ring-1 focus:ring-brand focus:outline-hidden transition"
                />
              </div>
              <button
                type="submit"
                disabled={sending || !text.trim()}
                className="rounded-xl bg-brand px-5 py-2.5 text-xs font-bold text-white shadow-sm hover:bg-brand-dark active:scale-95 disabled:opacity-50 disabled:pointer-events-none transition flex items-center gap-1.5 cursor-pointer"
              >
                {sending ? (
                  <span>Đang gửi...</span>
                ) : (
                  <>
                    <span>Gửi</span>
                    <span>➤</span>
                  </>
                )}
              </button>
            </form>
          </>
        ) : (
          <div className="grid h-full place-items-center text-center text-xs text-gray-400 p-8">
            <div className="flex flex-col items-center gap-2">
              <span className="text-3xl">💬</span>
              <p className="font-semibold text-gray-600">
                Hộp thư trò chuyện trực tiếp
              </p>
              <p className="text-gray-400 max-w-xs">
                Chọn một cuộc hội thoại từ danh sách bên trái để bắt đầu nhắn
                tin trao đổi.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
