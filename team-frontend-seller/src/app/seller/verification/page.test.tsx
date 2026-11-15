import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";
import { getVerificationStatus } from "@/lib/gateway/verification";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

import SellerVerificationPage from "./page";

class GuestRedirect extends Error {}

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  useRouter: () => ({ refresh: vi.fn() }),
  usePathname: () => "/seller/verification",
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new GuestRedirect("guest");
  }),
  redirectGuestToLoginFromHeader: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/verification", () => ({
  getVerificationStatus: vi.fn(),
  createKycUploadUrl: vi.fn(),
  submitKyc: vi.fn(),
}));
vi.mock("@/features/seller/verification/actions", () => ({
  createKycUploadUrlAction: vi.fn(),
  submitKycAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const seller = { id: "seller-1", name: "S", scopes: ["listing.write"] };

async function renderPage(searchParams: Record<string, string> = {}) {
  const ui = await SellerVerificationPage({
    searchParams: Promise.resolve(searchParams),
  });
  return render(<>{ui}</>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(seller as never);
  vi.mocked(hasScope).mockReturnValue(true);
});

describe("/seller/verification page", () => {
  it("A seller who never submitted sees NOT_SUBMITTED and the form: status read for the caller id only", async () => {
    vi.mocked(getVerificationStatus).mockResolvedValue({
      status: VerificationStatus.NOT_SUBMITTED,
      statusText: "",
      badge: false,
      rejectReasonCode: "",
    } as never);
    const { container } = await renderPage({ user_id: "someone-else" });
    expect(screen.getByText("Chưa gửi hồ sơ")).toBeInTheDocument();
    expect(container.querySelectorAll('input[type="file"]')).toHaveLength(1);
    expect(getVerificationStatus).toHaveBeenCalledTimes(1);
    expect(getVerificationStatus).toHaveBeenCalledWith("seller-1");
  });

  it("A pending submission hides the form", async () => {
    vi.mocked(getVerificationStatus).mockResolvedValue({
      status: VerificationStatus.PENDING,
      badge: false,
      rejectReasonCode: "",
    } as never);
    const { container } = await renderPage();
    expect(screen.getByText("Đang chờ duyệt")).toBeInTheDocument();
    expect(container.querySelectorAll('input[type="file"]')).toHaveLength(0);
  });

  it("Status read failure is shown as an error, not as not-submitted", async () => {
    vi.mocked(getVerificationStatus).mockRejectedValue(new Error("UNAVAILABLE"));
    const { container } = await renderPage();
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toHaveAttribute(
      "href",
      "/seller/verification",
    );
    expect(screen.queryByText("Chưa gửi hồ sơ")).toBeNull();
    expect(container.querySelectorAll('input[type="file"]')).toHaveLength(0);
    expect(container.querySelectorAll("form")).toHaveLength(0);
  });

  it("A guest is sent to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(
      SellerVerificationPage({ searchParams: Promise.resolve({}) }),
    ).rejects.toBeInstanceOf(GuestRedirect);
    expect(redirectGuestToLogin).toHaveBeenCalledWith(
      "/seller/verification",
      expect.anything(),
    );
    expect(getVerificationStatus).not.toHaveBeenCalled();
  });

  it("A buyer without seller scope cannot use the page", async () => {
    vi.mocked(hasScope).mockReturnValue(false);
    await SellerVerificationPage({ searchParams: Promise.resolve({}) });
    expect(hasScope).toHaveBeenCalledWith("listing.write");
    expect(getVerificationStatus).not.toHaveBeenCalled();
  });
});
