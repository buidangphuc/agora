import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getListing, listCategories } from "@/lib/gateway/listings";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

import SellerEditPage from "@/app/(shop)/seller/[id]/edit/page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
  usePathname: () => "/seller/l1/edit",
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("guest");
  }),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
  listCategories: vi.fn(),
}));
vi.mock("@/lib/requestCache", () => ({
  requestCache: <T,>(fn: T) => fn,
}));
vi.mock("@/features/listing/actions", () => ({
  saveListingAction: vi.fn(),
  magicListingAction: vi.fn(),
  getUploadUrlAction: vi.fn(),
}));
vi.mock("@/lib/analytics/ai", () => ({ trackAiInteraction: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

function listing(status: string, moderation?: Record<string, string>) {
  return {
    id: "l1000000000",
    title: "Phone",
    description: "d",
    price: 100,
    currency: "VND",
    status,
    sellerId: "s1",
    imageKeys: [],
    categoryId: "cat1",
    stock: 1,
    variants: [],
    moderation,
  };
}

async function open(status: string, moderation?: Record<string, string>) {
  vi.mocked(getListing).mockResolvedValue(listing(status, moderation) as never);
  const ui = await SellerEditPage({ params: Promise.resolve({ id: "l1" }) });
  return render(ui as never);
}

function saveButton() {
  return screen.queryByRole("button", { name: /Lưu thay đổi/ });
}

function titleDisabled(container: HTMLElement): boolean {
  const title = container.querySelector('input[name="title"]');
  if (!title) throw new Error("title input not rendered");
  return title.matches(":disabled");
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "s1",
    name: "Seller",
    scopes: ["listing.write"],
  } as never);
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(listCategories).mockResolvedValue([
    { id: "cat1", name: "Điện thoại" },
  ] as never);
});

describe("seller edit page moderation banner", () => {
  // Scenario: Moderation UI: Rejected listing shows reason and note
  it("shows the rejected reason and note", async () => {
    const { container } = await open("rejected", {
      reasonCode: "counterfeit",
      note: "Ảnh là hàng nhái",
      decidedAt: "2026-10-16T00:00:00Z",
    });
    expect(
      screen.getByText("Tin đăng bị từ chối: Hàng giả. Ảnh là hàng nhái"),
    ).toBeTruthy();
    expect(titleDisabled(container)).toBe(false);
    expect(saveButton()).toBeTruthy();
  });

  // Scenario: Moderation UI: Banned listing is read-only
  it("banned renders the form read-only", async () => {
    const { container } = await open("banned", {
      reasonCode: "counterfeit",
      note: "Ảnh là hàng nhái",
      decidedAt: "2026-10-16T00:00:00Z",
    });
    expect(container.textContent).toContain("Hàng giả");
    expect(container.textContent).toContain("Ảnh là hàng nhái");
    expect(titleDisabled(container)).toBe(true);
    expect(saveButton()).toBeNull();
    expect(container.querySelector('button[type="submit"]')).toBeNull();
  });

  // Scenario: Moderation UI: Held listing is explained
  it("held renders the explanation", async () => {
    const { container } = await open("held");
    expect(
      screen.getByText(
        "Tin đăng đang tạm ẩn vì tài khoản của bạn bị hạn chế. Tin sẽ hiển thị lại khi tài khoản được khôi phục.",
      ),
    ).toBeTruthy();
    expect(titleDisabled(container)).toBe(true);
    expect(saveButton()).toBeNull();
  });

  // Scenario: Moderation UI: Pending listing is explained
  it("pending shows the waiting banner", async () => {
    const { container } = await open("pending_review");
    expect(screen.getByText("Tin đăng đang chờ kiểm duyệt.")).toBeTruthy();
    expect(titleDisabled(container)).toBe(false);
  });

  // Scenario: Moderation UI: Unknown reason code is shown raw
  it("falls back to the raw reason code", async () => {
    await open("rejected", {
      reasonCode: "new_reason_x",
      note: "",
      decidedAt: "2026-10-16T00:00:00Z",
    });
    expect(screen.getByText(/new_reason_x/)).toBeTruthy();
  });

  // Scenario: Moderation UI: Screen flags never reach a seller surface (page side)
  it("never renders keyword flags on the edit page", async () => {
    const { container } = await open("pending_review");
    expect(container.textContent).not.toContain("keyword:");
  });

  it("shows no banner for a published listing", async () => {
    const { container } = await open("published");
    expect(container.textContent).not.toContain("Tin đăng đang chờ");
    expect(container.textContent).not.toContain("Tin đăng bị từ chối");
    expect(saveButton()).toBeTruthy();
  });
});
