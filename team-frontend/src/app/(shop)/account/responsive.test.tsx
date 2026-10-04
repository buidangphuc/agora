import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  AlertType,
  DigestFrequency,
} from "@/generated/platform/notification/v1/notification_pb.js";

// jsdom has no layout engine, so this checks the responsive contract of the
// markup (class structure) for every account route. Measured layout at 375px
// and 1280px is covered in the browser (see platform-e2e account.feature).
vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  useRouter: () => ({ refresh: vi.fn() }),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(() => ({ id: "u", name: "t", scopes: [] })),
}));
vi.mock("@/lib/gateway/addresses", () => ({
  listAddresses: vi.fn(async () => [
    {
      id: "a1",
      userId: "u",
      recipientName: "Nguyễn Văn A",
      phone: "0912345678",
      street: "1 Lê Lợi",
      ward: "",
      district: "",
      city: "TP. HCM",
      isDefault: false,
    },
  ]),
}));
vi.mock("@/lib/gateway/sessions", () => ({
  listSessions: vi.fn(async () => [
    {
      id: "s1",
      device: "Chrome",
      ip: "1.1.1.1",
      createdAt: "x",
      lastSeen: "y",
      revoked: false,
    },
  ]),
  listLoginHistory: vi.fn(async () => []),
}));
vi.mock("@/lib/gateway/verification", async (orig) => ({
  ...(await orig<typeof import("@/lib/gateway/verification")>()),
  getVerificationStatus: vi.fn(async () => ({
    status: 0,
    statusText: "Chưa xác minh",
    badge: false,
  })),
}));
vi.mock("@/lib/gateway/referral", () => ({
  getMyReferral: vi.fn(async () => ({
    code: "",
    invitedCount: 0,
    rewardsTotal: 0,
  })),
  listReferralRewards: vi.fn(async () => []),
}));
vi.mock("@/lib/gateway/engagement", () => ({
  listFollowedSellers: vi.fn(async () => [
    { sellerId: "s-1", displayName: "Shop A" },
  ]),
  listCollections: vi.fn(async () => [
    { id: "c1", userId: "u", name: "Mua sau", itemCount: 1 },
  ]),
  listFavoriteIds: vi.fn(async () => ({ ids: [], nextCursor: "", total: 0 })),
  listCollectionItems: vi.fn(async () => ({
    ids: [],
    nextCursor: "",
    total: 0,
  })),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(async () => null),
}));
vi.mock("@/lib/gateway/search", () => ({ searchListings: vi.fn() }));
vi.mock("@/lib/gateway/notification", () => ({
  listNotifications: vi.fn(async () => ({
    notifications: [],
    totalUnread: 0,
  })),
  listAlertSubscriptions: vi.fn(async () => [
    { id: "sub", listingId: "l1", type: AlertType.PRICE_DROP },
  ]),
  getNotificationPrefs: vi.fn(async () => ({
    typeEnabled: {},
    digestFreq: DigestFrequency.OFF,
  })),
}));
vi.mock("@/features/address/actions", () => ({}));
vi.mock("@/features/account/actions", () => ({}));
vi.mock("@/features/account/referral/actions", () => ({}));
vi.mock("@/features/account/verification/actions", () => ({}));
vi.mock("@/features/engagement/actions", () => ({}));
vi.mock("@/features/notification/actions", () => ({}));
vi.mock("@/features/account/FollowedFeed", () => ({
  FollowedFeed: () => null,
  FollowedFeedSkeleton: () => null,
}));
vi.mock("@/features/listing/ListingGrid", () => ({ ListingGrid: () => null }));

import AddressesPage from "./addresses/page";
import FollowingPage from "./following/page";
import ReferralPage from "./referral/page";
import SecurityPage from "./security/page";
import VerificationPage from "./verification/page";

import FavoritesPage from "../favorites/page";
import NotificationsPage from "../notifications/page";

type Page = () => Promise<React.JSX.Element>;

const SHELL_PAGES: [string, Page][] = [
  ["/account/addresses", () => AddressesPage() as never],
  ["/account/security", () => SecurityPage({}) as never],
  ["/account/verification", () => VerificationPage() as never],
  ["/account/referral", () => ReferralPage() as never],
  ["/account/following", () => FollowingPage({}) as never],
];

const WIDE_PAGES: [string, Page][] = [
  ["/favorites", () => FavoritesPage({}) as never],
  ["/notifications", () => NotificationsPage({}) as never],
];

beforeEach(() => vi.clearAllMocks());

describe("settings pages: menu beside content from lg, one scrollable row below", () => {
  for (const [route, load] of SHELL_PAGES) {
    it(route, async () => {
      const { container } = render(await load());
      const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
      expect(nav.className).toContain("lg:w-60");
      expect(nav.querySelector("ul")?.className).toContain("overflow-x-auto");
      const layout = nav.parentElement;
      expect(layout?.className).toContain("flex-col");
      expect(layout?.className).toContain("lg:flex-row");
      // The content column can shrink, so a wide child never widens the page.
      expect(nav.nextElementSibling?.className).toContain("min-w-0");
      expect(nav.nextElementSibling?.className).toContain("max-w-3xl");
      expect(container.querySelector("[style*='width']")).toBeNull();
    });
  }
});

describe("standalone pages are wide, single column", () => {
  for (const [route, load] of WIDE_PAGES) {
    it(route, async () => {
      const { container } = render(await load());
      expect(container.querySelector("section")?.className).toContain(
        "max-w-5xl",
      );
      expect(container.querySelector("[style*='width']")).toBeNull();
    });
  }
});

describe("tap targets: form controls and page buttons are at least 40px high", () => {
  for (const [route, load] of [...SHELL_PAGES, ...WIDE_PAGES]) {
    it(route, async () => {
      const { container } = render(await load());
      const controls = container.querySelectorAll(
        "input:not([type=checkbox]), select, main button, button",
      );
      for (const control of controls) {
        const ok =
          control.className.includes("min-h-10") ||
          // size="lg" buttons are 44px tall.
          control.className.includes("py-2.5");
        expect(ok, `${route}: ${control.outerHTML.slice(0, 120)}`).toBe(true);
      }
    });
  }

  it("the notification tab strip scrolls instead of widening the page", async () => {
    const { container } = render(await NotificationsPage({}));
    const tabs = container.querySelector("nav[aria-label='Tabs']");
    expect(tabs?.closest(".overflow-x-auto")).not.toBeNull();
  });
});
