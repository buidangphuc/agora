import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { deactivateShopVoucherAction } from "../actions";
import { VoucherTab } from "./VoucherTab";
import { voucherRow } from "./testRows";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
  usePathname: () => "/seller/marketing",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("../actions", () => ({
  createShopVoucherAction: vi.fn(),
  deactivateShopVoucherAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(deactivateShopVoucherAction).mockResolvedValue({ ok: true });
});

const rowFor = (code: string) =>
  screen.getByText(code).closest('[data-testid="voucher-row"]') as HTMLElement;

describe("VoucherTab", () => {
  it("Vouchers of every state are listed with used and quota: shows state label and used/quota per row", () => {
    render(
      <VoucherTab
        rows={[
          voucherRow({ id: "a", code: "ACT", used: 3, quota: 100 }),
          voucherRow({
            id: "s",
            code: "SCH",
            state: "scheduled",
            stateText: "Sắp diễn ra",
            used: 0,
            quota: 50,
          }),
          voucherRow({
            id: "e",
            code: "END",
            state: "ended",
            stateText: "Đã kết thúc",
            used: 7,
            quota: 7,
          }),
          voucherRow({
            id: "d",
            code: "OFF",
            active: false,
            state: "deactivated",
            stateText: "Đã tắt",
            used: 1,
            quota: 9,
          }),
        ]}
      />,
    );
    expect(screen.getAllByTestId("voucher-row")).toHaveLength(4);
    expect(within(rowFor("ACT")).getByText("Đang chạy")).toBeInTheDocument();
    expect(within(rowFor("ACT")).getByText("3/100")).toBeInTheDocument();
    expect(within(rowFor("SCH")).getByText("Sắp diễn ra")).toBeInTheDocument();
    expect(within(rowFor("END")).getByText("Đã kết thúc")).toBeInTheDocument();
    expect(within(rowFor("OFF")).getByText("Đã tắt")).toBeInTheDocument();
    expect(within(rowFor("OFF")).getByText("1/9")).toBeInTheDocument();
  });

  it("Deactivating an active voucher: confirming calls the action with only the voucher id", async () => {
    render(<VoucherTab rows={[voucherRow({ id: "v9", code: "GIAM10" })]} />);
    fireEvent.click(screen.getByRole("button", { name: "Tắt voucher" }));
    expect(deactivateShopVoucherAction).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Xác nhận" }));
    await vi.waitFor(() =>
      expect(deactivateShopVoucherAction).toHaveBeenCalledTimes(1),
    );
    expect(deactivateShopVoucherAction).toHaveBeenCalledWith("v9");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("cancelling the deactivate confirmation calls nothing", async () => {
    render(<VoucherTab rows={[voucherRow({ id: "v9", code: "GIAM10" })]} />);
    fireEvent.click(screen.getByRole("button", { name: "Tắt voucher" }));
    fireEvent.click(await screen.findByRole("button", { name: "Huỷ bỏ" }));
    expect(deactivateShopVoucherAction).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(within(rowFor("GIAM10")).getByText("Đang chạy")).toBeInTheDocument();
  });

  it("Deactivating an active voucher: a deactivated row offers no Tắt voucher control", () => {
    render(
      <VoucherTab
        rows={[
          voucherRow({
            id: "d",
            code: "OFF",
            active: false,
            state: "deactivated",
            stateText: "Đã tắt",
          }),
        ]}
      />,
    );
    expect(screen.queryByRole("button", { name: "Tắt voucher" })).toBeNull();
  });

  it("Deactivating an active voucher: a server refusal is shown in an alert", async () => {
    vi.mocked(deactivateShopVoucherAction).mockResolvedValue({
      ok: false,
      error: "Không có quyền hoặc voucher không tồn tại",
    });
    render(<VoucherTab rows={[voucherRow({ id: "v9", code: "GIAM10" })]} />);
    fireEvent.click(screen.getByRole("button", { name: "Tắt voucher" }));
    fireEvent.click(await screen.findByRole("button", { name: "Xác nhận" }));
    expect(
      await screen.findByText("Không có quyền hoặc voucher không tồn tại"),
    ).toBeInTheDocument();
  });

  it("Empty list shows an empty state: no rows, a Tạo voucher call to action, no alert", () => {
    render(<VoucherTab rows={[]} />);
    expect(screen.queryAllByTestId("voucher-row")).toHaveLength(0);
    expect(screen.getAllByText("Tạo voucher").length).toBeGreaterThan(0);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("375 px layout: the row list scrolls or stacks instead of overflowing the viewport", () => {
    render(<VoucherTab rows={[voucherRow()]} />);
    expect(screen.getByTestId("marketing-list").className).toMatch(
      /overflow-x-auto|flex-col|grid-cols-1|space-y/,
    );
  });
});
