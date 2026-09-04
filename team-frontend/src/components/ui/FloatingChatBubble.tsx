"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

export function FloatingChatBubble() {
  const pathname = usePathname();

  // Don't show floating chat bubble if already inside /chat to avoid clutter
  if (pathname.startsWith("/chat")) {
    return null;
  }

  return (
    <div className="fixed bottom-5 right-5 z-40 md:bottom-6 md:right-6 select-none group">
      <Link
        href="/chat"
        aria-label="Mở hộp thư tin nhắn và hỗ trợ"
        className="flex items-center gap-2 rounded-full bg-gradient-to-r from-orange-600 to-brand px-4 py-3 text-white shadow-lg shadow-orange-500/30 hover:shadow-orange-500/50 hover:scale-105 active:scale-95 transition-all duration-200"
      >
        <div className="relative flex items-center justify-center">
          <span className="text-xl leading-none">💬</span>
          {/* Active online pulse dot */}
          <span className="absolute -top-1 -right-1 flex h-2.5 w-2.5">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-yellow-300 opacity-75" />
            <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-yellow-400" />
          </span>
        </div>

        <span className="text-xs font-bold tracking-wide hidden sm:inline-block">
          Chat
        </span>

        {/* Floating tooltip on hover for desktop */}
        <span className="pointer-events-none absolute right-0 bottom-full mb-2 hidden md:group-hover:flex items-center whitespace-nowrap rounded-lg bg-gray-900 px-3 py-1.5 text-[11px] font-medium text-white shadow-md opacity-0 group-hover:opacity-100 transition-opacity">
          Nhắn tin với Người bán & Hỗ trợ
          <span className="absolute top-full right-6 -mt-1 border-4 border-transparent border-t-gray-900" />
        </span>
      </Link>
    </div>
  );
}
