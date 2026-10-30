import "./globals.css";

import type { ReactNode } from "react";

import { ToastProvider } from "@/components/ui/ToastProvider";

export const metadata = {
  title: "Kênh Người Bán | Marketplace",
  description: "Quản lý sản phẩm, đơn hàng và gian hàng của người bán.",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="vi">
      <body className="bg-surface-page text-text-primary flex flex-col min-h-screen font-sans antialiased">
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
