import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ request: vi.fn(), refresh: vi.fn() }));
vi.mock("./actions", () => ({ requestDeletionAction: h.request }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: h.refresh, push: vi.fn() }),
}));

import { DeleteAccountModal } from "./DeleteAccountModal";

beforeEach(() => {
  h.request.mockReset();
  h.refresh.mockReset();
});
afterEach(cleanup);

const field = () => screen.getByLabelText(/Mật khẩu/i) as HTMLInputElement;
const confirm = () => screen.getByRole("button", { name: /Xác nhận/i });

function mount(props: { onScheduled?: () => void; onClose?: () => void } = {}) {
  return render(
    <DeleteAccountModal
      isOpen
      onClose={props.onClose ?? (() => {})}
      onScheduled={props.onScheduled ?? (() => {})}
    />,
  );
}

describe("DeleteAccountModal", () => {
  it("PV18 confirm is disabled for an empty password", async () => {
    const user = userEvent.setup();
    mount();
    expect(confirm()).toBeDisabled();
    await user.click(confirm());
    expect(h.request).not.toHaveBeenCalled();
    await user.type(field(), "x");
    expect(confirm()).toBeEnabled();
  });

  it("PV17 wrong password keeps the modal open and clears the field", async () => {
    const user = userEvent.setup();
    const onScheduled = vi.fn();
    h.request.mockResolvedValue({ ok: false, error: "Mật khẩu không đúng." });
    mount({ onScheduled });
    await user.type(field(), "sai-mat-khau");
    await user.click(confirm());

    expect(await screen.findByText("Mật khẩu không đúng.")).toBeInTheDocument();
    expect(h.request).toHaveBeenCalledWith("sai-mat-khau");
    expect(field().value).toBe("");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(onScheduled).not.toHaveBeenCalled();
  });

  it("PV19 a blocker is shown with the prefix and the field is cleared", async () => {
    const user = userEvent.setup();
    const msg = "Chưa thể xóa tài khoản: còn đơn hàng đang mở";
    h.request.mockResolvedValue({ ok: false, error: msg });
    mount();
    await user.type(field(), "mat-khau-dung");
    await user.click(confirm());
    expect(await screen.findByText(msg)).toBeInTheDocument();
    expect(field().value).toBe("");
  });

  it("PV16 success clears the password and reports scheduling", async () => {
    const user = userEvent.setup();
    const onScheduled = vi.fn();
    h.request.mockResolvedValue({ ok: true });
    mount({ onScheduled });
    await user.type(field(), "mat-khau-dung");
    await user.click(confirm());
    await waitFor(() => expect(onScheduled).toHaveBeenCalledTimes(1));
    expect(h.request).toHaveBeenCalledWith("mat-khau-dung");
    expect(field().value).toBe("");
  });

  it("PV17 the password field is cleared when the modal is closed and reopened", async () => {
    const user = userEvent.setup();
    const { rerender } = mount();
    await user.type(field(), "bi-mat");
    rerender(
      <DeleteAccountModal
        isOpen={false}
        onClose={() => {}}
        onScheduled={() => {}}
      />,
    );
    rerender(
      <DeleteAccountModal isOpen onClose={() => {}} onScheduled={() => {}} />,
    );
    expect(field().value).toBe("");
  });

  it("PV25 blocks a double submit", async () => {
    const user = userEvent.setup();
    let release!: (v: unknown) => void;
    h.request.mockReturnValue(
      new Promise((r) => {
        release = r;
      }),
    );
    mount();
    await user.type(field(), "mat-khau-dung");
    const btn = confirm();
    await act(async () => {
      btn.click();
      btn.click();
    });
    expect(h.request).toHaveBeenCalledTimes(1);
    await act(async () => {
      release({ ok: true });
    });
    expect(h.request).toHaveBeenCalledTimes(1);
  });
});
