import { render, screen, within } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";
import {
  type ViewLoginEvent,
  type ViewSession,
  listLoginHistory,
  listSessions,
} from "@/lib/gateway/sessions";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
const refresh = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  useRouter: () => ({ refresh }),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/sessions", () => ({
  listSessions: vi.fn(),
  listLoginHistory: vi.fn(),
}));
vi.mock("@/features/account/actions", () => ({ revokeSessionAction: vi.fn() }));

import { revokeSessionAction } from "@/features/account/actions";
import SecurityPage from "./page";

const live: ViewSession = {
  id: "s1",
  device: "Chrome / macOS",
  ip: "10.0.0.1",
  createdAt: "01/10/2026",
  lastSeen: "04/10/2026",
  revoked: false,
};
const gone: ViewSession = {
  ...live,
  id: "s2",
  device: "Safari",
  revoked: true,
};

function events(n: number): ViewLoginEvent[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `e${i}`,
    ip: "10.0.0.1",
    userAgent: `Agent ${i}`,
    success: i % 2 === 0,
    createdAt: "04/10/2026",
  }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(listSessions).mockResolvedValue([live, gone]);
  vi.mocked(listLoginHistory).mockResolvedValue(events(3));
});

describe("/account/security", () => {
  it("redirects an anonymous visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await SecurityPage({});
    expect(redirect).toHaveBeenCalledWith("/login");
  });

  it("shows the shell, both sections and result Tags", async () => {
    render(await SecurityPage({}));
    expect(
      screen.getByRole("heading", { level: 1, name: "Bảo mật tài khoản" }),
    ).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Menu tài khoản" });
    expect(within(nav).getByRole("link", { name: "Bảo mật" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      screen.getByRole("heading", { name: "Phiên đăng nhập" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Lịch sử đăng nhập" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Thành công").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Thất bại").length).toBeGreaterThan(0);
    // The revoked row has a Tag and no button; the live row has one.
    expect(screen.getAllByRole("button", { name: "Thu hồi" })).toHaveLength(1);
    expect(screen.getByText("Đã thu hồi")).toBeInTheDocument();
  });

  it("renders Empty for an empty history", async () => {
    vi.mocked(listLoginHistory).mockResolvedValue([]);
    render(await SecurityPage({}));
    expect(screen.getByText("Chưa có lịch sử đăng nhập.")).toBeInTheDocument();
  });

  it("a failed sessions read shows an inline Alert with retry; history still renders", async () => {
    const user = setupUser();
    vi.mocked(listSessions).mockRejectedValue(new Error("down"));
    render(await SecurityPage({}));
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Không thể tải danh sách phiên đăng nhập.");
    expect(
      screen.getByRole("heading", { name: "Lịch sử đăng nhập" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Agent 0")).toBeInTheDocument();

    await user.click(within(alert).getByRole("button", { name: "Thử lại" }));
    expect(refresh).toHaveBeenCalled();
  });

  it("paginates the history by ?page= at 20 rows", async () => {
    vi.mocked(listLoginHistory).mockResolvedValue(events(45));
    render(await SecurityPage({ searchParams: { page: "3" } }));
    expect(screen.getByText("Agent 40")).toBeInTheDocument();
    expect(screen.queryByText("Agent 39")).toBeNull();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(within(nav).getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "href",
      "/account/security?page=2",
    );
    expect(within(nav).getByRole("link", { name: "Trang 3" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("revoking a session asks for confirmation, then toasts", async () => {
    const user = setupUser();
    vi.mocked(revokeSessionAction).mockResolvedValue({ ok: true });
    render(await SecurityPage({}));

    await user.click(screen.getByRole("button", { name: "Thu hồi" }));
    const dialog = screen.getByRole("dialog", {
      name: "Thu hồi phiên đăng nhập?",
    });
    expect(revokeSessionAction).not.toHaveBeenCalled();
    await user.click(
      within(dialog).getByRole("button", { name: "Thu hồi phiên" }),
    );

    expect(revokeSessionAction).toHaveBeenCalledWith("s1");
    expect(toast.success).toHaveBeenCalledWith("Đã thu hồi phiên đăng nhập.");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getAllByText("Đã thu hồi")).toHaveLength(2);
  });

  it("a failed revoke toasts the error and keeps the confirm open", async () => {
    const user = setupUser();
    vi.mocked(revokeSessionAction).mockResolvedValue({
      ok: false,
      error: "Thu hồi phiên thất bại.",
    });
    render(await SecurityPage({}));
    await user.click(screen.getByRole("button", { name: "Thu hồi" }));
    await user.click(screen.getByRole("button", { name: "Thu hồi phiên" }));
    expect(toast.error).toHaveBeenCalledWith("Thu hồi phiên thất bại.");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
