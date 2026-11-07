import "./globals.css";

import type { Metadata } from "next";
import type { ReactNode } from "react";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { ConsentBanner } from "@/features/consent/ConsentBanner";
import { AnalyticsProvider } from "@/features/tracking/AnalyticsProvider";
import { siteUrl } from "@/lib/seo";

export const metadata: Metadata = {
  metadataBase: new URL(siteUrl()),
  title: {
    default: "Agora | Sàn thương mại điện tử",
    template: "%s | Agora",
  },
  description:
    "Agora là sàn thương mại điện tử: mua sắm sản phẩm từ nhiều người bán, giá tốt, giao hàng nhanh.",
};

/**
 * Root layout: document + providers only, so tracking and toasts wrap every
 * route. Visible chrome lives in route groups: (shop) is the consumer shell
 * (header, banner, footer, floating chat), (checkout) the distraction-free one.
 */
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="vi">
      <body className="bg-surface-page text-text-primary flex flex-col min-h-screen font-sans antialiased">
        <AnalyticsProvider>
          <ToastProvider>{children}</ToastProvider>
        </AnalyticsProvider>
        <ConsentBanner />
      </body>
    </html>
  );
}
