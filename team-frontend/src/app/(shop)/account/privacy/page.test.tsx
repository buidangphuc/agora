import { render, screen, within } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  type ConsentEntry,
  getDeletionStatus,
  listConsents,
} from "@/lib/gateway/privacy";
import { getPrincipal } from "@/lib/gateway/session";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
const refresh = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  notFound: vi.fn(),
  useRouter: () => ({ refresh }),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/privacy", () => ({
  listConsents: vi.fn(),
  getDeletionStatus: vi.fn(),
}));
vi.mock("@/features/privacy/actions", () => ({
  saveConsentAction: vi.fn(),
  exportDataAction: vi.fn(),
  requestDeletionAction: vi.fn(),
  cancelDeletionAction: vi.fn(),
}));
vi.mock("@/features/welcome/WelcomePanel", () => ({
  WelcomePanel: () => null,
}));

import PrivacyPage, * as pageModule from "./page";

function records(n: number): ConsentEntry[] {
  return Array.from({ length: n }, (_, i) => ({
    purpose: "TERMS" as const,
    granted: true,
    version: `h${i}`,
    source: "account",
  }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(listConsents).mockResolvedValue({
    records: records(3),
    nextCursor: "",
  });
  vi.mocked(getDeletionStatus).mockResolvedValue({
    pending: false,
    eraseAfter: "",
  });
});

describe("/account/privacy", () => {
  it("PV2 PV29 redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(PrivacyPage({})).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Fprivacy",
    );
    expect(listConsents).not.toHaveBeenCalled();
    expect(getDeletionStatus).not.toHaveBeenCalled();
  });

  it("PV1 is force-dynamic and renders the shell with the privacy entry current", async () => {
    expect(pageModule.dynamic).toBe("force-dynamic");
    render(await PrivacyPage({}));
    expect(
      screen.getByRole("heading", { level: 1, name: "Quyền riêng tư" }),
    ).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    expect(
      within(nav).getByRole("link", { name: "Quyền riêng tư" }),
    ).toHaveAttribute("aria-current", "page");
  });

  it("PV3 a failed consent read keeps the other cards", async () => {
    const user = setupUser();
    vi.mocked(listConsents).mockRejectedValue(new Error("unavailable"));
    render(await PrivacyPage({}));
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Không thể tải lịch sử đồng ý.");
    expect(
      screen.getByText(
        "Xóa tài khoản sẽ xóa dữ liệu cá nhân của bạn sau thời gian chờ.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /đơn hàng/i })).toBeInTheDocument();

    await user.click(within(alert).getByRole("button", { name: "Thử lại" }));
    expect(refresh).toHaveBeenCalled();
  });

  it("PV4 lists consents newest first, 20 per page", async () => {
    vi.mocked(listConsents).mockResolvedValue({
      records: records(25),
      nextCursor: "",
    });
    const { unmount } = render(await PrivacyPage({}));
    const items = screen.getAllByText(/^Phiên bản h\d+$/);
    expect(items).toHaveLength(20);
    expect(items[0]).toHaveTextContent("Phiên bản h0");
    expect(screen.queryByText("Phiên bản h20")).toBeNull();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "href",
      "/account/privacy?page=2",
    );
    unmount();

    render(
      await PrivacyPage({ searchParams: Promise.resolve({ page: "2" }) }),
    );
    expect(screen.getAllByText(/^Phiên bản h\d+$/)).toHaveLength(5);
    expect(screen.getByText("Phiên bản h20")).toBeInTheDocument();
    expect(screen.queryByText("Phiên bản h19")).toBeNull();
  });

  it("PV9 empty history shows the empty state", async () => {
    vi.mocked(listConsents).mockResolvedValue({ records: [], nextCursor: "" });
    render(await PrivacyPage({}));
    expect(screen.getByText("Chưa có lịch sử đồng ý.")).toBeInTheDocument();
  });

  it("PV15 export card links orders and chats", async () => {
    render(await PrivacyPage({}));
    expect(screen.getByRole("link", { name: /đơn hàng/i })).toHaveAttribute(
      "href",
      "/account/orders",
    );
    expect(
      screen.getByRole("link", { name: /tin nhắn|trò chuyện|chat/i }),
    ).toHaveAttribute("href", "/chat");
  });
});
