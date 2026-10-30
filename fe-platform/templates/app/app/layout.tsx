import type { ReactNode } from "react";
import "./globals.css";

export const viewport = {
  width: "device-width",
  initialScale: 1,
  viewportFit: "cover",
};

export const metadata = {
  title: "FE Platform",
  description: "AI-ready frontend platform template",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body className="bg-surface-page font-sans text-base text-text-primary">
        {children}
      </body>
    </html>
  );
}
