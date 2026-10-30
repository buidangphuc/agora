import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal } from "@/lib/gateway/session";

class GuestRedirect extends Error {}

vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
  usePathname: () => "/seller",
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new GuestRedirect("guest-redirect");
  }),
  redirectGuestToLoginFromHeader: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));

const restricted = { id: "buyer123456", name: "Restricted", scopes: [] };

const PAGES: Array<{
  name: string;
  path: string;
  load: () => Promise<{ default: (props: never) => unknown }>;
  props: Record<string, unknown>;
}> = [
  {
    name: "/seller",
    path: "/seller",
    load: () => import("./(dashboard)/page"),
    props: { searchParams: Promise.resolve({}) },
  },
  {
    name: "/seller/new",
    path: "/seller/new",
    load: () => import("./new/page"),
    props: {},
  },
  {
    name: "/seller/ads",
    path: "/seller/ads",
    load: () => import("./ads/page"),
    props: {},
  },
  {
    name: "/seller/plans",
    path: "/seller/plans",
    load: () => import("./plans/page"),
    props: {},
  },
  {
    name: "/seller/shop",
    path: "/seller/shop",
    load: () => import("./shop/page"),
    props: {},
  },
  {
    name: "/seller/wallet",
    path: "/seller/wallet",
    load: () => import("./wallet/page"),
    props: { searchParams: Promise.resolve({}) },
  },
  {
    name: "/seller/bundles",
    path: "/seller/bundles",
    load: () => import("./bundles/page"),
    props: {},
  },
  {
    name: "/seller/analytics",
    path: "/seller/analytics",
    load: () => import("./analytics/page"),
    props: { searchParams: Promise.resolve({}) },
  },
  {
    name: "/seller/orders",
    path: "/seller/orders",
    load: () => import("./orders/(list)/page"),
    props: { searchParams: Promise.resolve({}) },
  },
  {
    name: "/seller/{id}/edit",
    path: "/seller/abc/edit",
    load: () => import("./[id]/edit/page"),
    props: { params: Promise.resolve({ id: "abc" }) },
  },
];

async function run(page: (typeof PAGES)[number]) {
  const mod = await page.load();
  return (mod.default as (p: unknown) => Promise<unknown>)(page.props);
}

beforeEach(async () => {
  vi.clearAllMocks();
  const session = await import("@/lib/gateway/session");
  vi.mocked(session.hasScope).mockImplementation((scope: string) =>
    Boolean(vi.mocked(getPrincipal)()?.scopes.includes(scope)),
  );
});

describe("Users UI: Seller pages do not send a principal without the listing.write scope to login", () => {
  describe.each(PAGES)("$name", (page) => {
    it("signed-in principal without listing.write is never redirected", async () => {
      vi.mocked(getPrincipal).mockReturnValue(restricted);
      const result = await run(page);
      expect(result).toBeNull();
      expect(redirectGuestToLogin).not.toHaveBeenCalled();
      expect(redirect).not.toHaveBeenCalled();
    });

    it("no principal calls redirectGuestToLogin with the page's own path", async () => {
      vi.mocked(getPrincipal).mockReturnValue(null);
      await expect(run(page)).rejects.toBeInstanceOf(GuestRedirect);
      expect(redirectGuestToLogin).toHaveBeenCalledTimes(1);
      expect(vi.mocked(redirectGuestToLogin).mock.calls[0][0]).toBe(page.path);
    });
  });
});
