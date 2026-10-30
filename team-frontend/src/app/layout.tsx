import "./globals.css";

import type { ReactNode } from "react";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { ConsentBanner } from "@/features/consent/ConsentBanner";
import { AnalyticsProvider } from "@/features/tracking/AnalyticsProvider";

export const metadata = {
  title: "Marketplace Showcase | Nền Tảng Thương Mại Điện Tử Polyrepo",
  description:
    "Dự án thương mại điện tử kiến trúc Polyrepo, Microservices gRPC, Event-driven Kafka, OpenSearch và Next.js SSR.",
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
