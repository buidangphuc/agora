import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AdCampaignForm } from "./AdCampaignForm";
import { BundleManager } from "./BundleManager";
import { createAdCampaignAction, createBundleAction } from "./actions";

vi.mock("./actions", () => ({
  createAdCampaignAction: vi.fn(),
  createBundleAction: vi.fn(),
}));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const listings = [
  { id: "l1", title: "Áo" },
  { id: "l2", title: "Quần" },
];

beforeEach(() => vi.clearAllMocks());

describe("BundleManager", () => {
  const bundles = [
    {
      id: "b1",
      sellerId: "s",
      title: "Combo hè",
      listingIds: ["l1", "l2"],
      bundlePrice: 300000,
      createdAt: "01/10/2026",
    },
  ];

  it("lists existing bundles in a Table", () => {
    render(<BundleManager listings={listings} bundles={bundles} />);
    const table = screen.getByRole("table", { name: "Combo hiện có" });
    expect(within(table).getByText("Combo hè")).toBeInTheDocument();
  });

  it("shows the field error and an error toast, and sends nothing when fewer than 2 are picked", () => {
    render(<BundleManager listings={listings} bundles={[]} />);
    fireEvent.change(screen.getByLabelText(/Tên combo/), {
      target: { value: "X" },
    });
    fireEvent.change(screen.getByLabelText(/Giá combo/), {
      target: { value: "100" },
    });
    fireEvent.click(screen.getByLabelText("Áo"));
    fireEvent.click(screen.getByRole("button", { name: "Tạo combo" }));
    expect(
      screen.getByText("Chọn ít nhất 2 sản phẩm cho combo."),
    ).toBeInTheDocument();
    expect(toastError).toHaveBeenCalledWith(
      "Chọn ít nhất 2 sản phẩm cho combo.",
    );
    expect(createBundleAction).not.toHaveBeenCalled();
  });

  it("creates a bundle: pending, success toast, form reset", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(createBundleAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<BundleManager listings={listings} bundles={[]} />);
    fireEvent.change(screen.getByLabelText(/Tên combo/), {
      target: { value: "X" },
    });
    fireEvent.change(screen.getByLabelText(/Giá combo/), {
      target: { value: "100" },
    });
    fireEvent.click(screen.getByLabelText("Áo"));
    fireEvent.click(screen.getByLabelText("Quần"));
    fireEvent.click(screen.getByRole("button", { name: "Tạo combo" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Tạo combo" })).toBeDisabled(),
    );
    expect(createBundleAction).toHaveBeenCalledWith("X", ["l1", "l2"], 100);
    resolve({ ok: true, data: { bundle: bundles[0] } });
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã tạo combo"),
    );
    expect(screen.getByLabelText(/Tên combo/)).toHaveValue("");
    expect(screen.getByLabelText("Áo")).not.toBeChecked();
  });

  it("shows the server error in an Alert and a toast", async () => {
    vi.mocked(createBundleAction).mockResolvedValue({
      ok: false,
      error: "Trùng tên",
    });
    render(<BundleManager listings={listings} bundles={[]} />);
    fireEvent.change(screen.getByLabelText(/Tên combo/), {
      target: { value: "X" },
    });
    fireEvent.change(screen.getByLabelText(/Giá combo/), {
      target: { value: "100" },
    });
    fireEvent.click(screen.getByLabelText("Áo"));
    fireEvent.click(screen.getByLabelText("Quần"));
    fireEvent.click(screen.getByRole("button", { name: "Tạo combo" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Trùng tên"),
    );
    expect(toastError).toHaveBeenCalledWith("Trùng tên");
  });
});

describe("AdCampaignForm", () => {
  it("validates budget and bid inline", () => {
    render(<AdCampaignForm listings={listings} />);
    fireEvent.click(screen.getByRole("button", { name: "Chạy quảng cáo" }));
    expect(screen.getByText("Ngân sách phải lớn hơn 0.")).toBeInTheDocument();
    expect(screen.getByText("Giá thầu phải lớn hơn 0.")).toBeInTheDocument();
    expect(createAdCampaignAction).not.toHaveBeenCalled();
  });

  it("launches a campaign and lists it in a Table", async () => {
    vi.mocked(createAdCampaignAction).mockResolvedValue({
      ok: true,
      data: {
        campaign: {
          id: "campaign-12345",
          budget: 1000,
          bid: 10,
          statusText: "Đang chạy",
        } as never,
      },
    });
    render(<AdCampaignForm listings={listings} />);
    fireEvent.change(screen.getByLabelText(/Ngân sách/), {
      target: { value: "1000" },
    });
    fireEvent.change(screen.getByLabelText(/Giá thầu/), {
      target: { value: "10" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Chạy quảng cáo" }));
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Đã tạo chiến dịch quảng cáo"),
    );
    expect(createAdCampaignAction).toHaveBeenCalledWith("l1", 1000, 10);
    expect(screen.getByText("#campaign")).toBeInTheDocument();
    expect(screen.getByLabelText(/Ngân sách/)).toHaveValue(null);
  });

  it("surfaces a server failure", async () => {
    vi.mocked(createAdCampaignAction).mockResolvedValue({
      ok: false,
      error: "Hết ngân sách",
    });
    render(<AdCampaignForm listings={listings} />);
    fireEvent.change(screen.getByLabelText(/Ngân sách/), {
      target: { value: "1000" },
    });
    fireEvent.change(screen.getByLabelText(/Giá thầu/), {
      target: { value: "10" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Chạy quảng cáo" }));
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Hết ngân sách"),
    );
    expect(toastError).toHaveBeenCalledWith("Hết ngân sách");
  });
});
