import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewDispute } from "@/lib/gateway/disputes";
import { setupUser } from "@/test/user";

import { EscalateDisputeModal } from "./EscalateDisputeModal";
import { createDisputeAction } from "./actions";

vi.mock("./actions", () => ({
  createDisputeAction: vi.fn(),
  resolveDisputeAction: vi.fn(),
}));
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

// Contract under test (implementer creates EscalateDisputeModal.tsx):
//   <EscalateDisputeModal isOpen onClose orderId sellerId returnId
//      onCreated={(d: ViewDispute) => void} />
//   textarea data-testid="dispute-reason", submit button data-testid="dispute-submit"
//   (label "Gửi khiếu nại"); validation / refusal text rendered as plain text.
// On success: onCreated(dispute) then onClose(); on refusal: message shown, reason kept.

const created: ViewDispute = {
  id: "d1",
  orderId: "o1",
  claimantId: "b1",
  defendantId: "seller-1",
  reason: "Người bán từ chối vô lý",
  resolution: "",
  returnId: "r1",
  evidenceUrls: [],
  status: "OPEN",
  statusText: "Đang chờ xử lý",
  ruling: "",
  rulingText: "",
  createdAt: "2026-10-22T00:00:00.000Z",
};

function setup() {
  const onCreated = vi.fn();
  const onClose = vi.fn();
  render(
    <EscalateDisputeModal
      isOpen
      onClose={onClose}
      orderId="o1"
      sellerId="seller-1"
      returnId="r1"
      onCreated={onCreated}
    />,
  );
  return { onCreated, onClose };
}

function typeReason(value: string) {
  fireEvent.change(screen.getByTestId("dispute-reason"), {
    target: { value },
  });
}

beforeEach(() => vi.clearAllMocks());

describe("EscalateDisputeModal", () => {
  it("[Escalation creates a dispute] sends the trimmed reason with order, seller and return ids once", async () => {
    vi.mocked(createDisputeAction).mockResolvedValue({
      ok: true,
      data: created,
    });
    const { onCreated } = setup();
    typeReason("  Người bán từ chối vô lý  ");
    await setupUser().click(screen.getByTestId("dispute-submit"));

    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(created));
    expect(createDisputeAction).toHaveBeenCalledTimes(1);
    expect(createDisputeAction).toHaveBeenCalledWith({
      orderId: "o1",
      defendantId: "seller-1",
      returnId: "r1",
      reason: "Người bán từ chối vô lý",
    });
  });

  it.each(["", "   ", "\n\t "])(
    "[Blank reason is refused locally] refuses a blank reason without a call (%j)",
    async (value) => {
      setup();
      typeReason(value);
      await setupUser().click(screen.getByTestId("dispute-submit"));
      expect(
        screen.getByText("Vui lòng nhập lý do khiếu nại."),
      ).toBeInTheDocument();
      expect(createDisputeAction).not.toHaveBeenCalled();
    },
  );

  it("[Reason at the length limit] accepts 1000 characters and refuses 1001", async () => {
    vi.mocked(createDisputeAction).mockResolvedValue({
      ok: true,
      data: created,
    });
    const user = setupUser();
    const first = setup();
    typeReason("a".repeat(1001));
    await user.click(screen.getByTestId("dispute-submit"));
    expect(screen.getByText("Lý do tối đa 1000 ký tự.")).toBeInTheDocument();
    expect(createDisputeAction).not.toHaveBeenCalled();
    expect(first.onCreated).not.toHaveBeenCalled();

    typeReason("a".repeat(1000));
    await user.click(screen.getByTestId("dispute-submit"));
    await waitFor(() => expect(createDisputeAction).toHaveBeenCalledTimes(1));
    expect(vi.mocked(createDisputeAction).mock.calls[0][0].reason).toBe(
      "a".repeat(1000),
    );
  });

  it.each([
    "Yêu cầu trả hàng này chưa thể khiếu nại.",
    "Yêu cầu trả hàng này đã có khiếu nại đang xử lý.",
    "Bạn không có quyền khiếu nại đơn hàng này.",
    "Không gửi được khiếu nại. Vui lòng thử lại sau.",
  ])(
    "[Server says the return cannot be escalated / A dispute already exists / Buyer is not a party / Gateway outage on create] shows a refusal message, keeps the reason and adds no entry: %s",
    async (message) => {
      vi.mocked(createDisputeAction).mockResolvedValue({
        ok: false,
        error: message,
      });
      const { onCreated, onClose } = setup();
      typeReason("Người bán từ chối vô lý");
      await setupUser().click(screen.getByTestId("dispute-submit"));

      expect(await screen.findByText(message)).toBeInTheDocument();
      expect(screen.getByTestId("dispute-reason")).toHaveValue(
        "Người bán từ chối vô lý",
      );
      expect(onCreated).not.toHaveBeenCalled();
      expect(onClose).not.toHaveBeenCalled();
      expect(screen.getByTestId("dispute-submit")).toBeEnabled();
    },
  );

  it("[Double click sends one request] is single-flight", async () => {
    let resolve: (v: Awaited<ReturnType<typeof createDisputeAction>>) => void =
      () => {};
    vi.mocked(createDisputeAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    const { onCreated } = setup();
    typeReason("Người bán từ chối vô lý");
    const submit = screen.getByTestId("dispute-submit");
    fireEvent.click(submit);
    fireEvent.click(submit);

    await waitFor(() =>
      expect(screen.getByTestId("dispute-submit")).toBeDisabled(),
    );
    expect(createDisputeAction).toHaveBeenCalledTimes(1);

    resolve({ ok: true, data: created });
    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1));
    expect(createDisputeAction).toHaveBeenCalledTimes(1);
  });
});
