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
      <header className="border-b border-border-subtle bg-surface-card shadow-card">
        <div className="mx-auto flex w-full max-w-page items-center justify-between gap-3 px-4 py-3 md:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <span
              aria-hidden="true"
              className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-action-primary text-sm font-bold text-text-inverse"
            >
              QT
            </span>
            <p className="truncate text-base font-bold text-text-primary md:text-lg">
              Quản trị sàn
            </p>
          </div>
          <div className="flex min-w-0 items-center gap-3">
            {userName ? (
              <span
                className="hidden max-w-48 truncate text-sm text-text-secondary sm:block"
                data-testid="admin-user"
              >
                {userName}
              </span>
            ) : null}
            <form action={logoutAction} className="shrink-0 whitespace-nowrap">
              <Button type="submit" variant="secondary" size="sm">
                Đăng xuất
              </Button>
            </form>
          </div>
        </div>
        <nav aria-label="Quản trị" className="border-t border-border-subtle">
          <div className="mx-auto w-full max-w-page">
            <ul className="flex gap-1 overflow-x-auto px-4 py-2 md:px-6">
              {visibleNav().map((item) => (
                <li key={item.href} className="shrink-0 whitespace-nowrap">
                  <Link href={item.href} variant="ghost" size="sm">
                    {item.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        </nav>
      </header>
      <main className="mx-auto w-full max-w-page flex-1 px-4 py-6 md:px-6 md:py-10">
        {children}
      </main>
    </>
  );
}
