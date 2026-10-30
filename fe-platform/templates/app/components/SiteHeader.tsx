import { Button, Link } from "@fe-platform/ui";
import { logout } from "../app/(public)/login/actions";
import { getToken } from "../lib/session";

export async function SiteHeader() {
  const signedIn = Boolean(await getToken());
  return (
    <header className="border-b border-border-subtle bg-surface-card">
      <nav
        aria-label="Main"
        className="mx-auto flex w-full max-w-5xl flex-wrap items-center gap-6 px-4 md:px-6"
      >
        <span className="font-semibold">FE Platform</span>
        <Link href="/products">Products</Link>
        <Link href="/shop/cart">Cart</Link>
        {signedIn ? (
          <>
            <Link href="/dashboard">Orders</Link>
            <form action={logout} className="ml-auto">
              <Button type="submit" variant="ghost" size="sm">
                Sign out
              </Button>
            </form>
          </>
        ) : (
          <span className="ml-auto">
            <Link href="/login">Sign in</Link>
          </span>
        )}
      </nav>
    </header>
  );
}
