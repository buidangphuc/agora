import { logoutAction } from "@/features/auth/actions";

import { NavIcon } from "./nav";

export function SellerLogoutButton({ compact = false }: { compact?: boolean }) {
  return (
    <form action={logoutAction}>
      <button
        type="submit"
        aria-label={compact ? "Đăng xuất" : undefined}
        className="flex min-h-touch w-full items-center justify-center gap-2 rounded-xl px-3 text-xs font-medium text-text-secondary transition hover:bg-surface-muted hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
      >
        <NavIcon name="logout" className="h-4 w-4" />
        {!compact && <span>Đăng xuất</span>}
      </button>
    </form>
  );
}
