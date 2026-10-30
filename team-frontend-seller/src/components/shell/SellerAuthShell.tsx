import NextLink from "next/link";
import type { ReactNode } from "react";

import { buyerAppUrl } from "@/lib/apps";

export function SellerAuthShell({ children }: { children: ReactNode }) {
  return (
    <>
      <header className="border-b border-border-subtle bg-surface-card">
        <div className="mx-auto flex max-w-page items-center justify-between gap-4 px-4 py-3">
          <NextLink
            href="/seller"
            className="text-lg font-bold text-text-primary"
          >
            Kênh Người Bán
          </NextLink>
          <a
            href={buyerAppUrl("/")}
            className="text-sm text-text-secondary hover:text-text-primary"
          >
            Về trang mua sắm
          </a>
        </div>
      </header>
      <main className="mx-auto w-full max-w-page flex-1 px-4 py-5">
        {children}
      </main>
    </>
  );
}
