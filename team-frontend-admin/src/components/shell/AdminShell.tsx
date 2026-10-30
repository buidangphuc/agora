import type { ReactNode } from "react";

import { Button } from "@/components/ui/Button";
import { Link } from "@/components/ui/Link";
import { logoutAction } from "@/features/auth/actions";

import { visibleNav } from "./nav";

export function AdminShell({
  userName,
  children,
}: {
  userName: string;
  children: ReactNode;
}) {
  return (
    <>
      <header className="border-b border-border-subtle bg-surface-card">
        <div className="mx-auto flex w-full max-w-page flex-wrap items-center justify-between gap-3 px-4 py-3">
          <p className="text-base font-bold text-text-primary">Quản trị sàn</p>
          <div className="flex items-center gap-3">
            {userName ? (
              <span
                className="text-sm text-text-secondary"
                data-testid="admin-user"
              >
                {userName}
              </span>
            ) : null}
            <form action={logoutAction}>
              <Button type="submit" variant="secondary" size="sm">
                Đăng xuất
              </Button>
            </form>
          </div>
        </div>
        <nav
          aria-label="Quản trị"
          className="mx-auto w-full max-w-page overflow-x-auto px-4 pb-2"
        >
          <ul className="flex gap-1">
            {visibleNav().map((item) => (
              <li key={item.href}>
                <Link href={item.href} variant="subtle">
                  {item.label}
                </Link>
              </li>
            ))}
          </ul>
        </nav>
      </header>
      <main className="mx-auto w-full max-w-page flex-1 px-4 py-6">
        {children}
      </main>
    </>
  );
}
