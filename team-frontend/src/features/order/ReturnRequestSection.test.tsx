import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { createDisputeAction } from "@/features/dispute/actions";
import { ReturnStatus } from "@/generated/platform/order/v1/order_pb.js";
import type { ViewDispute } from "@/lib/gateway/disputes";
import { setupUser } from "@/test/user";

import { ReturnRequestSection } from "./ReturnRequestSection";
import { ReturnStateProvider } from "./ReturnState";
import { createReturnRequestAction } from "./actions";

vi.mock("./actions", () => ({
  createReturnRequestAction: vi.fn(),
}));

vi.mock("@/features/dispute/actions", () => ({
  createDisputeAction: vi.fn(),
  resolveDisputeAction: vi.fn(),
}));

const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => toast,
}));

const pendingReturn = {
  id: "r1",
  orderId: "o1",
  reason: "hỏng",
  refundAmount: 50000,
  status: ReturnStatus.PENDING,
  statusText: "Chờ duyệt",
  canEscalate: false,
};

const rejectedReturn = {
  ...pendingReturn,
  status: ReturnStatus.REJECTED,
  statusText: "Đã từ chối",
  canEscalate: true,
};

const dispute = (over: Partial<ViewDispute> = {}): ViewDispute => ({
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
  ...over,
});

const ESCALATE = { name: "Khiếu nại lên Agora" };

function escalable(
  props: Partial<React.ComponentProps<typeof ReturnRequestSection>> = {},
) {
  return (
    <ReturnRequestSection
      orderId="o1"
      orderTotal={50000}
      sellerId="seller-1"
      initialReturns={[rejectedReturn]}
      {...props}
    />
  );
}

beforeEach(() => vi.clearAllMocks());

async function openModal(user: ReturnType<typeof setupUser>) {
  await user.click(screen.getByRole("button", { name: "Yêu cầu trả hàng" }));
}

describe("ReturnRequestSection", () => {
  it("shows Empty without a request action when the order is not eligible", () => {
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} />);
    expect(screen.getByTestId("return-section")).toBeInTheDocument();
    expect(screen.getByText("Chưa có yêu cầu trả hàng")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Yêu cầu trả hàng" }),
    ).toBeNull();
  });

  it("submits a return request from the Modal and then shows its status", async () => {
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: true,
      data: pendingReturn,
    });

    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(screen.getByTestId("return-reason"), "defective");
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() =>
      expect(createReturnRequestAction).toHaveBeenCalledWith(
        "o1",
        "Sản phẩm bị lỗi / hư hỏng",
        50000,
      ),
    );
    expect(await screen.findByTestId("return-status")).toHaveTextContent(
      "Chờ duyệt",
    );
    expect(toast.success).toHaveBeenCalled();
    expect(screen.queryByTestId("return-submit")).toBeNull();
  });

  it("blocks an invalid form with inline errors and calls no action", async () => {
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.click(screen.getByTestId("return-submit"));
    expect(
      screen.getByText("Vui lòng chọn lý do trả hàng."),
    ).toBeInTheDocument();

    await user.selectOptions(
      screen.getByTestId("return-reason"),
      "changed_mind",
    );
    await user.clear(screen.getByTestId("return-amount"));
    await user.type(screen.getByTestId("return-amount"), "60000");
    await user.click(screen.getByTestId("return-submit"));
    expect(
      screen.getByText("Số tiền hoàn không được vượt quá tổng đơn hàng."),
    ).toBeInTheDocument();
    expect(createReturnRequestAction).not.toHaveBeenCalled();
    expect(screen.getByTestId("return-submit")).toBeInTheDocument();
  });

  it("requires a description for 'other'", async () => {
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(screen.getByTestId("return-reason"), "other");
    await user.click(screen.getByTestId("return-submit"));
    expect(screen.getByText("Vui lòng mô tả lý do.")).toBeInTheDocument();
    await user.type(screen.getByTestId("return-reason-detail"), "hỏng");
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: true,
      data: pendingReturn,
    });
    await user.click(screen.getByTestId("return-submit"));
    await waitFor(() =>
      expect(createReturnRequestAction).toHaveBeenCalledWith(
        "o1",
        "hỏng",
        50000,
      ),
    );
  });

  it("keeps the Modal open and toasts the error when the action fails", async () => {
    vi.mocked(createReturnRequestAction).mockResolvedValue({
      ok: false,
      error: "boom",
    });
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(
      screen.getByTestId("return-reason"),
      "changed_mind",
    );
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("boom"));
    expect(screen.getByTestId("return-submit")).toBeEnabled();
    expect(screen.queryByTestId("return-status")).toBeNull();
  });

  it("disables the form and the submit button while pending", async () => {
    let resolve: (
      v: Awaited<ReturnType<typeof createReturnRequestAction>>,
    ) => void = () => {};
    vi.mocked(createReturnRequestAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    render(<ReturnRequestSection orderId="o1" orderTotal={50000} canRequest />);
    const user = setupUser();
    await openModal(user);
    await user.selectOptions(
      screen.getByTestId("return-reason"),
      "changed_mind",
    );
    await user.click(screen.getByTestId("return-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("return-submit")).toBeDisabled(),
    );
    expect(screen.getByTestId("return-reason")).toBeDisabled();
    expect(screen.getByTestId("return-amount")).toBeDisabled();

    resolve({ ok: true, data: pendingReturn });
    expect(await screen.findByTestId("return-status")).toBeInTheDocument();
  });

  it.each([
    [ReturnStatus.PENDING, "Chờ duyệt"],
    [ReturnStatus.APPROVED, "Đã duyệt"],
    [ReturnStatus.REJECTED, "Đã từ chối"],
    [ReturnStatus.REFUNDED, "Đã hoàn tiền"],
  ])("lists a return in status %s with no refund button", (status, text) => {
    render(
      <ReturnRequestSection
        orderId="o1"
        orderTotal={50000}
        initialReturns={[{ ...pendingReturn, status, statusText: text }]}
      />,
    );
    expect(screen.getByTestId("return-status")).toHaveTextContent(text);
    expect(screen.queryByRole("button", { name: /Hoàn tiền/ })).toBeNull();
    expect(screen.queryByTestId("return-refund")).toBeNull();
  });

  it("lists every return of the order", () => {
    render(
      <ReturnRequestSection
        orderId="o1"
        orderTotal={50000}
        initialReturns={[
          pendingReturn,
          { ...pendingReturn, id: "r2", statusText: "Đã duyệt" },
        ]}
      />,
    );
    expect(screen.getAllByTestId("return-status")).toHaveLength(2);
  });

  it("shows a return created elsewhere through the shared provider", () => {
    render(
      <ReturnStateProvider initialReturns={[pendingReturn]}>
        <ReturnRequestSection orderId="o1" orderTotal={50000} />
      </ReturnStateProvider>,
    );
    expect(screen.getByTestId("return-status")).toHaveTextContent("Chờ duyệt");
  });

  describe("escalation to Agora", () => {
    it("[Button is shown on an escalable return] shows the escalate button when canEscalate and no open dispute", () => {
      render(escalable({ disputes: [] }));
      expect(screen.getAllByRole("button", ESCALATE)).toHaveLength(1);
    });

    it("[Button is hidden when the return cannot be escalated] hides the escalate button when canEscalate is false", () => {
      render(
        escalable({
          initialReturns: [{ ...rejectedReturn, canEscalate: false }],
          disputes: [],
        }),
      );
      expect(screen.queryByRole("button", ESCALATE)).toBeNull();
    });

    it.each(["OPEN", "INVESTIGATING"] as const)(
      "[Button is hidden while a dispute is open for the return] hides the escalate button while a dispute is %s",
      (status) => {
        render(escalable({ disputes: [dispute({ status })] }));
        expect(screen.queryByRole("button", ESCALATE)).toBeNull();
        expect(screen.getAllByTestId("dispute-item")).toHaveLength(1);
      },
    );

    it("[Button is hidden while a dispute is open for the return] an open dispute of another return does not hide it", () => {
      render(escalable({ disputes: [dispute({ returnId: "r-other" })] }));
      expect(screen.getAllByRole("button", ESCALATE)).toHaveLength(1);
    });

    it("[A closed dispute does not hide the button forever] shows the escalate button after a closed dispute", () => {
      render(
        escalable({
          disputes: [
            dispute({ id: "d2", status: "REJECTED", statusText: "Đã từ chối" }),
            dispute({
              id: "d3",
              status: "RESOLVED",
              statusText: "Đã giải quyết",
            }),
          ],
        }),
      );
      expect(screen.getAllByRole("button", ESCALATE)).toHaveLength(1);
    });

    it("[Escalation creates a dispute] shows the created dispute as Đang chờ xử lý and hides the button", async () => {
      vi.mocked(createDisputeAction).mockResolvedValue({
        ok: true,
        data: dispute(),
      });
      render(escalable({ disputes: [] }));
      const user = setupUser();
      await user.click(screen.getByRole("button", ESCALATE));
      await user.type(
        screen.getByTestId("dispute-reason"),
        "Người bán từ chối vô lý",
      );
      await user.click(screen.getByTestId("dispute-submit"));

      await waitFor(() =>
        expect(createDisputeAction).toHaveBeenCalledWith({
          orderId: "o1",
          defendantId: "seller-1",
          returnId: "r1",
          reason: "Người bán từ chối vô lý",
        }),
      );
      expect(createDisputeAction).toHaveBeenCalledTimes(1);
      const items = await screen.findAllByTestId("dispute-item");
      expect(items).toHaveLength(1);
      expect(items[0]).toHaveTextContent("Đang chờ xử lý");
      expect(screen.queryByRole("button", ESCALATE)).toBeNull();
    });

    it("[Server says the return cannot be escalated] a refusal adds no dispute and keeps the button", async () => {
      vi.mocked(createDisputeAction).mockResolvedValue({
        ok: false,
        error: "Yêu cầu trả hàng này chưa thể khiếu nại.",
      });
      render(escalable({ disputes: [] }));
      const user = setupUser();
      await user.click(screen.getByRole("button", ESCALATE));
      await user.type(screen.getByTestId("dispute-reason"), "lý do");
      await user.click(screen.getByTestId("dispute-submit"));

      expect(
        await screen.findByText("Yêu cầu trả hàng này chưa thể khiếu nại."),
      ).toBeInTheDocument();
      expect(screen.queryAllByTestId("dispute-item")).toHaveLength(0);
    });

    it("[Dispute list failure leaves the order page intact] shows the list error and still lists the returns", () => {
      render(escalable({ disputes: [], disputesFailed: true }));
      expect(
        screen.getByText("Không tải được danh sách khiếu nại."),
      ).toBeInTheDocument();
      expect(screen.getByTestId("return-status")).toHaveTextContent(
        "Đã từ chối",
      );
    });

    it("[Order has no disputes] shows the empty dispute section by default", () => {
      render(escalable());
      expect(screen.getByText("Chưa có khiếu nại")).toBeInTheDocument();
    });
  });
});
