import type { ReactNode } from "react";
import "./globals.css";

export const metadata = { title: "Consumer example" };

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body className="bg-surface-page font-sans text-sm text-text-primary">
        {children}
      </body>
    </html>
  );
}
