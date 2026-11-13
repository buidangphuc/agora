import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ShopProfileForm } from "./ShopProfileForm";
import { upsertStorefrontAction } from "./actions";

vi.mock("./actions", () => ({
  upsertStorefrontAction: vi.fn(),
  searchMyListingsAction: vi.fn(),
}));
vi.mock("@/features/listing/actions", () => ({ getUploadUrlAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const EMPTY = {
  displayName: "",
  bannerUrl: "",
  tagline: "",
  featuredListingIds: [] as string[],
  theme: "default",
};
const initial = (over: Partial<typeof EMPTY> = {}) => ({ ...EMPTY, ...over });
const FULL = initial({
  displayName: "Hoa Mai",
  bannerUrl: "https://cdn.test/b.png",
  tagline: "Hoa tươi mỗi ngày",
  featuredListingIds: ["a", "b"],
  theme: "festive",
});
const TITLES = { a: "Hoa hồng", b: "Hoa cúc" };
const tagline = () => screen.getByLabelText(/Tagline/);

const field = () => screen.getByLabelText(/Tên gian hàng/);
const save = () =>
  fireEvent.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

beforeEach(() => vi.clearAllMocks());

describe("ShopProfileForm", () => {
  it("is pre-filled with the current name (name field)", () => {
    render(<ShopProfileForm initial={initial({ displayName: "Cửa hàng Hoa Mai" })} titles={{}} />);
    expect(field()).toHaveValue("Cửa hàng Hoa Mai");
  });

  it("saves a new name: pending, then a success toast", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(upsertStorefrontAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<ShopProfileForm initial={initial()} titles={{}} />);
    fireEvent.change(field(), { target: { value: "Nhà Sách An Nhiên" } });
    save();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Lưu thay đổi" }),
      ).toBeDisabled(),
    );
    expect(
      screen.getByRole("button", { name: "Lưu thay đổi" }),
    ).toHaveAttribute("aria-busy", "true");
    expect(upsertStorefrontAction).toHaveBeenCalledWith(
      expect.objectContaining({ displayName: "Nhà Sách An Nhiên" }),
    );
    resolve({ ok: true, data: { displayName: "Nhà Sách An Nhiên" } });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã lưu thông tin gian hàng"),
    );
  });

  it("rejects a blank name inline and sends nothing", () => {
    render(<ShopProfileForm initial={initial({ displayName: "Cũ" })} titles={{}} />);
    fireEvent.change(field(), { target: { value: "   " } });
    save();
    expect(field()).toHaveAttribute("aria-invalid", "true");
    const describedBy = field().getAttribute("aria-describedby") ?? "";
    expect(document.getElementById(describedBy)).toHaveTextContent(
      "Nhập tên gian hàng.",
    );
    expect(upsertStorefrontAction).not.toHaveBeenCalled();
  });

  it("rejects a name longer than 80 characters", () => {
    render(<ShopProfileForm initial={initial()} titles={{}} />);
    fireEvent.change(field(), { target: { value: "x".repeat(81) } });
    save();
    expect(
      screen.getByText("Tên gian hàng tối đa 80 ký tự."),
    ).toBeInTheDocument();
    expect(upsertStorefrontAction).not.toHaveBeenCalled();
  });

  it("accepts exactly 80 characters", async () => {
    vi.mocked(upsertStorefrontAction).mockResolvedValue({ ok: true });
    render(<ShopProfileForm initial={initial()} titles={{}} />);
    fireEvent.change(field(), { target: { value: "x".repeat(80) } });
    save();
    await waitFor(() => expect(upsertStorefrontAction).toHaveBeenCalled());
  });

  it("shows a server error in an Alert and a toast", async () => {
    vi.mocked(upsertStorefrontAction).mockResolvedValue({
      ok: false,
      error: "slug taken",
    });
    render(<ShopProfileForm initial={initial({ displayName: "A" })} titles={{}} />);
    save();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("slug taken"),
    );
    expect(toastError).toHaveBeenCalledWith("slug taken");
  });

  it("is pre-filled with the current storefront", () => {
    render(<ShopProfileForm initial={FULL} titles={TITLES} />);
    expect(screen.getByAltText("Banner gian hàng")).toHaveAttribute(
      "src",
      "https://cdn.test/b.png",
    );
    expect(tagline()).toHaveValue("Hoa tươi mỗi ngày");
    const rose = screen.getByText("Hoa hồng");
    const daisy = screen.getByText("Hoa cúc");
    expect(
      rose.compareDocumentPosition(daisy) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.getByLabelText(/Giao diện/)).toHaveValue("festive");
  });

  it("saves the full storefront with the changed tagline", async () => {
    vi.mocked(upsertStorefrontAction).mockResolvedValue({
      ok: true,
      data: { displayName: "Hoa Mai" },
    });
    render(<ShopProfileForm initial={FULL} titles={TITLES} />);
    fireEvent.change(tagline(), { target: { value: "Mới" } });
    save();
    await waitFor(() => expect(upsertStorefrontAction).toHaveBeenCalledTimes(1));
    expect(upsertStorefrontAction).toHaveBeenCalledWith({
      displayName: "Hoa Mai",
      bannerUrl: "https://cdn.test/b.png",
      tagline: "Mới",
      featuredListingIds: ["a", "b"],
      theme: "festive",
    });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã lưu thông tin gian hàng"),
    );
  });

  it("rejects an invalid tagline inline and sends nothing", () => {
    render(<ShopProfileForm initial={FULL} titles={TITLES} />);
    fireEvent.change(tagline(), { target: { value: "x".repeat(121) } });
    save();
    expect(screen.getByText("Tagline tối đa 120 ký tự.")).toBeInTheDocument();
    expect(upsertStorefrontAction).not.toHaveBeenCalled();
  });

  it("server rejection keeps the input", async () => {
    vi.mocked(upsertStorefrontAction).mockResolvedValue({
      ok: false,
      error: "Sản phẩm nổi bật không hợp lệ.",
    });
    render(<ShopProfileForm initial={FULL} titles={TITLES} />);
    fireEvent.change(tagline(), { target: { value: "Đang gõ dở" } });
    save();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        "Sản phẩm nổi bật không hợp lệ.",
      ),
    );
    expect(tagline()).toHaveValue("Đang gõ dở");
    expect(screen.getByText("Hoa hồng")).toBeInTheDocument();
    expect(screen.getByText("Hoa cúc")).toBeInTheDocument();
    expect(toastSuccess).not.toHaveBeenCalled();
  });
});
