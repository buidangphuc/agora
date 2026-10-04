import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingForm } from "./ListingForm";
import {
  getUploadUrlAction,
  magicListingAction,
  saveListingAction,
} from "./actions";

vi.mock("./actions", () => ({
  saveListingAction: vi.fn(),
  magicListingAction: vi.fn(),
  getUploadUrlAction: vi.fn(),
}));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const categories = [{ id: "cat1", name: "Điện thoại" }];

function fill(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(new RegExp(label)), {
    target: { value },
  });
}

function fillValid() {
  fill("Tên sản phẩm", "Phone");
  fireEvent.change(screen.getByLabelText(/Ngành hàng/), {
    target: { value: "cat1" },
  });
  fill("Giá bán", "5000000");
  fill("Kho hàng", "3");
}

const submit = () =>
  fireEvent.submit(
    screen
      .getByRole("button", { name: "Đăng bán ngay" })
      .closest("form") as HTMLFormElement,
  );

beforeEach(() => vi.clearAllMocks());

describe("ListingForm validation", () => {
  it("announces a missing title and sends no request", () => {
    render(<ListingForm categories={categories} submitLabel="Đăng bán ngay" />);
    submit();
    const title = screen.getByLabelText(/Tên sản phẩm/);
    expect(title).toHaveAttribute("aria-invalid", "true");
    const describedBy = title.getAttribute("aria-describedby") ?? "";
    expect(
      document.getElementById(describedBy.split(" ")[0]),
    ).toHaveTextContent("Tiêu đề bắt buộc.");
    expect(saveListingAction).not.toHaveBeenCalled();
  });

  it("validates a field on blur", () => {
    render(<ListingForm categories={categories} />);
    const price = screen.getByLabelText(/Giá bán/);
    fireEvent.change(price, { target: { value: "0" } });
    expect(price).not.toHaveAttribute("aria-invalid");
    fireEvent.blur(price);
    expect(price).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("Giá không hợp lệ.")).toBeInTheDocument();
  });

  it("requires a category and non-negative stock", () => {
    render(<ListingForm categories={categories} submitLabel="Đăng bán ngay" />);
    fill("Tên sản phẩm", "Phone");
    fill("Giá bán", "10");
    fill("Kho hàng", "-1");
    submit();
    expect(screen.getByText("Chọn ngành hàng.")).toBeInTheDocument();
    expect(screen.getByText("Tồn kho không hợp lệ.")).toBeInTheDocument();
    expect(saveListingAction).not.toHaveBeenCalled();
  });
});

describe("ListingForm submit", () => {
  it("is pending and non-repeatable, then toasts and shows the success Result", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(saveListingAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<ListingForm categories={categories} submitLabel="Đăng bán ngay" />);
    fillValid();
    submit();

    const button = screen.getByRole("button", { name: "Đăng bán ngay" });
    await waitFor(() => expect(button).toBeDisabled());
    expect(button).toHaveAttribute("aria-busy", "true");
    submit();
    expect(saveListingAction).toHaveBeenCalledTimes(1);
    // All sections' controls are disabled while pending.
    expect(screen.getByLabelText(/Tên sản phẩm/)).toBeDisabled();

    const sent = vi.mocked(saveListingAction).mock.calls[0][1] as FormData;
    expect(sent.get("title")).toBe("Phone");
    expect(sent.get("categoryId")).toBe("cat1");
    expect(sent.get("price")).toBe("5000000");
    expect(sent.get("imageKeys")).toBe("[]");
    expect(sent.get("variants")).toBe("[]");

    resolve({ ok: true, message: "✓ Đã đăng bán thành công", id: "l1" });
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith("Đã lưu"));
    expect(screen.getByText("✓ Đã đăng bán thành công")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem danh sách" })).toHaveAttribute(
      "href",
      "/seller",
    );
  });

  it("edit mode stays on the form with a success Alert", async () => {
    vi.mocked(saveListingAction).mockResolvedValue({
      ok: true,
      message: "✓ Đã cập nhật thành công",
      id: "l1",
    });
    render(
      <ListingForm
        listingId="l1"
        categories={categories}
        submitLabel="Lưu thay đổi"
        defaults={{
          id: "l1",
          title: "Phone",
          price: 5,
          stock: 1,
          categoryId: "cat1",
        }}
      />,
    );
    fireEvent.submit(
      screen
        .getByRole("button", { name: "Lưu thay đổi" })
        .closest("form") as HTMLFormElement,
    );
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledWith("Đã lưu"));
    expect(screen.getByText("✓ Đã cập nhật thành công")).toBeInTheDocument();
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue("Phone");
  });

  it("surfaces a server error, keeps the values and re-enables submit", async () => {
    vi.mocked(saveListingAction).mockResolvedValue({
      ok: false,
      message: "Lỗi: máy chủ",
      error: "Lỗi: máy chủ",
    });
    render(<ListingForm categories={categories} submitLabel="Đăng bán ngay" />);
    fillValid();
    submit();
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Lỗi: máy chủ"),
    );
    expect(toastError).toHaveBeenCalledWith("Lỗi: máy chủ");
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue("Phone");
    expect(screen.getByRole("button", { name: "Đăng bán ngay" })).toBeEnabled();
  });

  it("maps server field errors onto the field until it is edited", async () => {
    vi.mocked(saveListingAction).mockResolvedValue({
      ok: false,
      message: "Giá không hợp lệ.",
      error: "Giá không hợp lệ.",
      fieldErrors: { price: "Giá không hợp lệ." },
    });
    render(<ListingForm categories={categories} submitLabel="Đăng bán ngay" />);
    fillValid();
    submit();
    const price = screen.getByLabelText(/Giá bán/);
    await waitFor(() => expect(price).toHaveAttribute("aria-invalid", "true"));
    fill("Giá bán", "6000000");
    expect(price).not.toHaveAttribute("aria-invalid");
  });

  it("has a sticky action bar on mobile", () => {
    render(<ListingForm categories={categories} submitLabel="Đăng bán ngay" />);
    const bar = screen.getByRole("button", { name: "Đăng bán ngay" })
      .parentElement as HTMLElement;
    expect(bar).toHaveClass("sticky", "bottom-0");
    expect(within(bar).getByRole("link", { name: "Huỷ" })).toHaveAttribute(
      "href",
      "/seller",
    );
  });
});

describe("Magic Listing", () => {
  const suggestion = {
    generatedTitle: "Phone Pro - Chính hãng",
    generatedDescription: "Mô tả AI",
    suggestedCategoryId: "cat1",
    suggestedPriceMin: 4000000,
    suggestedPriceMax: 5000000,
    highlightTags: ["Chính Hãng"],
  };

  it("needs a typed title and never injects a default one", () => {
    render(<ListingForm categories={categories} />);
    expect(screen.getByRole("button", { name: "Tạo gợi ý" })).toBeDisabled();
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue("");
  });

  it("applies suggestions only on request", async () => {
    vi.mocked(magicListingAction).mockResolvedValue({
      ok: true,
      data: suggestion,
    });
    render(<ListingForm categories={categories} />);
    fill("Tên sản phẩm", "Phone");
    fill("Mô tả chi tiết", "Mô tả của tôi");
    fireEvent.click(screen.getByRole("button", { name: "Tạo gợi ý" }));

    await screen.findByTestId("magic-suggestion");
    expect(magicListingAction).toHaveBeenCalledWith("Phone", "");
    // Suggestion shown, form untouched.
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue("Phone");
    expect(screen.getByLabelText(/Mô tả chi tiết/)).toHaveValue(
      "Mô tả của tôi",
    );

    fireEvent.click(screen.getByRole("button", { name: "Áp dụng: Tiêu đề" }));
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue(
      "Phone Pro - Chính hãng",
    );
    expect(screen.getByLabelText(/Mô tả chi tiết/)).toHaveValue(
      "Mô tả của tôi",
    );
  });

  it("Áp dụng tất cả fills title, description and price", async () => {
    vi.mocked(magicListingAction).mockResolvedValue({
      ok: true,
      data: suggestion,
    });
    render(<ListingForm categories={categories} />);
    fill("Tên sản phẩm", "Phone");
    fireEvent.click(screen.getByRole("button", { name: "Tạo gợi ý" }));
    await screen.findByTestId("magic-suggestion");
    fireEvent.click(screen.getByRole("button", { name: "Áp dụng tất cả" }));
    expect(screen.getByLabelText(/Tên sản phẩm/)).toHaveValue(
      "Phone Pro - Chính hãng",
    );
    expect(screen.getByLabelText(/Mô tả chi tiết/)).toHaveValue("Mô tả AI");
    expect(screen.getByLabelText(/Giá bán/)).toHaveValue(4000000);
  });

  it("a failure shows an Alert with retry and leaves the form editable", async () => {
    vi.mocked(magicListingAction)
      .mockResolvedValueOnce({
        ok: false,
        error: "AI tạm thời không phản hồi.",
      })
      .mockResolvedValueOnce({ ok: true, data: suggestion });
    render(<ListingForm categories={categories} />);
    fill("Tên sản phẩm", "Phone");
    fireEvent.click(screen.getByRole("button", { name: "Tạo gợi ý" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("AI tạm thời không phản hồi.");
    expect(toastError).toHaveBeenCalledWith("AI tạm thời không phản hồi.");
    expect(screen.getByLabelText(/Tên sản phẩm/)).toBeEnabled();

    fireEvent.click(within(alert).getByRole("button", { name: "Thử lại" }));
    await screen.findByTestId("magic-suggestion");
    expect(magicListingAction).toHaveBeenCalledTimes(2);
  });
});

describe("variants and uploads", () => {
  it("serialises named variants into the hidden input", () => {
    render(
      <ListingForm
        categories={categories}
        defaults={{ variants: [{ id: "v1", name: "Đỏ", stock: 2 }] }}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Thêm phân loại" }));
    const hidden = document.querySelector(
      'input[name="variants"]',
    ) as HTMLInputElement;
    // The blank row is not sent.
    expect(JSON.parse(hidden.value)).toEqual([
      { id: "v1", name: "Đỏ", stock: 2 },
    ]);
  });

  it("rejects non-image files", async () => {
    render(<ListingForm categories={categories} />);
    const input = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    fireEvent.change(input, {
      target: { files: [new File(["x"], "a.txt", { type: "text/plain" })] },
    });
    await screen.findByText("Chỉ chấp nhận file hình ảnh.");
    expect(getUploadUrlAction).not.toHaveBeenCalled();
  });
});
