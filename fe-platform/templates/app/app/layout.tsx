import type { ReactNode } from "react";

export const metadata = {
  title: "FE Platform",
  description: "AI-ready frontend platform template",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
