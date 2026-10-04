import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { toneStyles } from "@/components/ui/tones";
import { getPrincipal } from "@/lib/gateway/session";
import {
  VerificationStatus,
  getVerificationStatus,
} from "@/lib/gateway/verification";
import { setupUser } from "@/test/user";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/verification", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/gateway/verification")>()),
  getVerificationStatus: vi.fn(),
  submitKyc: vi.fn(),
}));
vi.mock("@/features/account/verification/actions", () => ({
  submitKycAction: vi.fn(),
}));

import { submitKycAction } from "@/features/account/verification/actions";
import VerificationPage from "./page";

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
});

function status(s: VerificationStatus, statusText: string, badge = false) {
  vi.mocked(getVerificationStatus).mockResolvedValue({
    status: s,
    statusText,
    badge,
  });
}

describe("/account/verification status Tag", () => {
  const cases: [VerificationStatus, string, keyof typeof toneStyles][] = [
    [VerificationStatus.VERIFIED, "Đã xác minh", "success"],
    [VerificationStatus.PENDING, "Đang chờ duyệt", "warning"],
    [VerificationStatus.REJECTED, "Bị từ chối", "danger"],
    [VerificationStatus.UNSPECIFIED, "Chưa xác minh", "neutral"],
  ];
  for (const [s, text, tone] of cases) {
    it(`${text} uses the ${tone} tone and states the status as text`, async () => {
      status(s, text);
      render(await VerificationPage());
      const tag = screen.getByText(text);
      expect(tag.className).toContain(toneStyles[tone].soft);
      expect(tag.className).not.toContain("bg-primary");
    });
  }

  it("shows the verified badge line only when badge is true", async () => {
    status(VerificationStatus.VERIFIED, "Đã xác minh", true);
    const { unmount } = render(await VerificationPage());
    expect(screen.getByText("Huy hiệu đã xác minh")).toBeInTheDocument();
    unmount();

    status(VerificationStatus.PENDING, "Đang chờ duyệt", false);
    render(await VerificationPage());
    expect(screen.queryByText("Huy hiệu đã xác minh")).toBeNull();
  });

  it("redirects an anonymous visitor to /login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    status(VerificationStatus.UNSPECIFIED, "Chưa xác minh");
    await VerificationPage();
    expect(redirect).toHaveBeenCalledWith("/login");
  });
});

describe("SubmitKycForm", () => {
  it("every control has an accessible name", async () => {
    status(VerificationStatus.UNSPECIFIED, "Chưa xác minh");
    render(await VerificationPage());
    expect(screen.getByLabelText("Loại giấy tờ").tagName).toBe("SELECT");
    const ref = screen.getByLabelText("Mã tham chiếu tài liệu");
    expect(ref.tagName).toBe("INPUT");
    expect(ref).toHaveAccessibleDescription(
      "Nhập số giấy tờ hoặc khóa tệp đã tải lên.",
    );
  });

  it("disables submit until the reference is non-empty, then submits and clears", async () => {
    const user = setupUser();
    vi.mocked(submitKycAction).mockResolvedValue({ ok: true });
    status(VerificationStatus.UNSPECIFIED, "Chưa xác minh");
    render(await VerificationPage());

    const submit = screen.getByRole("button", { name: "Gửi hồ sơ xác minh" });
    expect(submit).toBeDisabled();
    const ref = screen.getByLabelText("Mã tham chiếu tài liệu");
    await user.type(ref, "0123456789");
    expect(submit).toBeEnabled();

    await user.click(submit);
    expect(submitKycAction).toHaveBeenCalledWith("national_id", "0123456789");
    expect(toast.success).toHaveBeenCalledWith("Đã gửi hồ sơ xác minh.");
    expect(ref).toHaveValue("");
  });

  it("shows an error toast when the submit fails", async () => {
    const user = setupUser();
    vi.mocked(submitKycAction).mockResolvedValue({
      ok: false,
      error: "Hồ sơ không hợp lệ.",
    });
    status(VerificationStatus.UNSPECIFIED, "Chưa xác minh");
    render(await VerificationPage());
    await user.type(screen.getByLabelText("Mã tham chiếu tài liệu"), "x");
    await user.click(
      screen.getByRole("button", { name: "Gửi hồ sơ xác minh" }),
    );
    expect(toast.error).toHaveBeenCalledWith("Hồ sơ không hợp lệ.");
  });
});
