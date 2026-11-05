import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SellerAccessDenied } from "./SellerAccessDenied";
import { SellerOnboarding } from "./SellerOnboarding";
import { acceptSellerTerms } from "./onboarding";
import { SELLER_TERMS_VERSION } from "./onboardingTerms";

vi.mock("./onboarding", () => ({ acceptSellerTerms: vi.fn() }));
vi.mock("@/features/auth/actions", () => ({ logoutAction: vi.fn() }));

const ACCEPT = "Đồng ý và trở thành Người Bán";

beforeEach(() => vi.clearAllMocks());

describe("SellerOnboarding", () => {
  it("[Seller onboarding UI: a buyer sees the onboarding screen] shows terms text, version and the accept button", () => {
    render(<SellerOnboarding />);
    expect(SELLER_TERMS_VERSION).toBeTruthy();
    expect(
      screen.getByText(new RegExp(SELLER_TERMS_VERSION)),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: ACCEPT })).toBeEnabled();
    expect(screen.queryByText("Cần tài khoản Người Bán")).toBeNull();
  });

  it("[Seller onboarding UI: the accept button is disabled while pending] button disabled while pending", async () => {
    let release: (v: { error: string }) => void = () => {};
    vi.mocked(acceptSellerTerms).mockImplementation(
      () =>
        new Promise((r) => {
          release = r;
        }),
    );
    render(<SellerOnboarding />);
    const button = screen.getByRole("button", { name: ACCEPT });
    fireEvent.click(button);
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(acceptSellerTerms).toHaveBeenCalledTimes(1);
    expect(acceptSellerTerms).toHaveBeenCalledWith(SELLER_TERMS_VERSION);
    await act(async () => {
      release({ error: "Không thể hoàn tất, thử lại sau" });
    });
    expect(screen.getByRole("button", { name: ACCEPT })).toBeEnabled();
  });

  it("[Seller onboarding UI: an unknown terms version shows an error] error result shows a visible line and re-enables the button", async () => {
    vi.mocked(acceptSellerTerms).mockResolvedValue({
      error: "Không thể xác nhận điều khoản",
    });
    render(<SellerOnboarding />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: ACCEPT }));
    });
    expect(
      screen.getByText("Không thể xác nhận điều khoản"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: ACCEPT })).toBeEnabled();
  });

  it("[Seller onboarding UI: a restricted account sees no onboarding button] denied result swaps to the denied notice without the button", async () => {
    vi.mocked(acceptSellerTerms).mockResolvedValue({ error: "denied" });
    render(<SellerOnboarding />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: ACCEPT }));
    });
    expect(screen.queryByRole("button", { name: ACCEPT })).toBeNull();
    expect(screen.queryByText("denied")).toBeNull();
    expect(
      screen.getByRole("link", { name: "Về trang mua sắm" }),
    ).toBeInTheDocument();
  });

  it("[Seller onboarding UI: the screens offer the shopping and logout actions] onboarding and denied notice offer shopping and logout", () => {
    const { unmount } = render(<SellerOnboarding />);
    expect(
      screen.getByRole("link", { name: "Về trang mua sắm" }),
    ).toHaveAttribute("href", "http://localhost:3000/");
    expect(
      screen.getByRole("button", { name: "Đăng xuất" }),
    ).toBeInTheDocument();
    unmount();
    render(<SellerAccessDenied restricted />);
    expect(
      screen.getByRole("link", { name: "Về trang mua sắm" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Đăng xuất" }),
    ).toBeInTheDocument();
  });
});
