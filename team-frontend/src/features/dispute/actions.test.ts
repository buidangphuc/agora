import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { type ViewDispute, createDispute } from "@/lib/gateway/disputes";
import { getPrincipal } from "@/lib/gateway/session";

import { createDisputeAction } from "./actions";

vi.mock("@/lib/gateway/disputes", () => ({
  createDispute: vi.fn(),
  listOrderDisputes: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));

// Contract under test (implementer creates src/features/dispute/actions.ts):
//   "use server"
//   createDisputeAction({ orderId, defendantId, returnId, reason })
//     => ActionResult<ViewDispute>  (ok: true carries data = the dispute)
// Error text is the proposal copy table; err.message is never forwarded.

const dispute: ViewDispute = {
  id: "d1",
  orderId: "o1",
  claimantId: "buyer-1",
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

const CREATE_INPUT = {
  orderId: "o1",
  defendantId: "seller-1",
  returnId: "r1",
  reason: "Người bán từ chối vô lý",
};

const MSG = {
  cannot: "Yêu cầu trả hàng này chưa thể khiếu nại.",
  exists: "Yêu cầu trả hàng này đã có khiếu nại đang xử lý.",
  perm: "Bạn không có quyền khiếu nại đơn hàng này.",
  notFound: "Không tìm thấy đơn hàng hoặc yêu cầu trả hàng này.",
  reload: "Không gửi được khiếu nại. Vui lòng tải lại trang và thử lại.",
  outage: "Không gửi được khiếu nại. Vui lòng thử lại sau.",
  transition: "Không thể chuyển khiếu nại sang trạng thái này.",
  audit:
    "Hệ thống kiểm toán tạm thời không khả dụng. Chưa có thay đổi nào được lưu.",
  timeout:
    "Chưa rõ thao tác đã được lưu hay chưa. Vui lòng tải lại trang để xem trạng thái hiện tại.",
};

function revalidatedOrderPage(): boolean {
  return vi
    .mocked(revalidatePath)
    .mock.calls.some(
      ([p]) => p === "/account/orders/o1" || p === "/account/orders/[id]",
    );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "admin-1",
    name: "root",
    scopes: ["admin"],
  });
  vi.mocked(createDispute).mockResolvedValue({ status: "ok", dispute });
});

describe("createDisputeAction", () => {
  // Scenario: Escalation creates a dispute
  it("createDisputeAction sends order, seller and return ids", async () => {
    const res = await createDisputeAction(CREATE_INPUT);
    expect(res).toEqual({ ok: true, data: dispute });
    expect(createDispute).toHaveBeenCalledTimes(1);
    expect(createDispute).toHaveBeenCalledWith(CREATE_INPUT);
    expect(revalidatedOrderPage()).toBe(true);
  });

  // Scenario: Server says the return cannot be escalated
  it("maps FAILED_PRECONDITION to the cannot-escalate message", async () => {
    vi.mocked(createDispute).mockResolvedValue({
      status: "failed_precondition",
    });
    expect(await createDisputeAction(CREATE_INPUT)).toEqual({
      ok: false,
      error: MSG.cannot,
    });
  });

  // Scenario: A dispute already exists for the return
  it("maps ALREADY_EXISTS and revalidates the order", async () => {
    vi.mocked(createDispute).mockResolvedValue({ status: "already_exists" });
    expect(await createDisputeAction(CREATE_INPUT)).toEqual({
      ok: false,
      error: MSG.exists,
    });
    expect(revalidatedOrderPage()).toBe(true);
  });

  // Scenario: Buyer is not a party
  it("maps PERMISSION_DENIED", async () => {
    vi.mocked(createDispute).mockResolvedValue({ status: "permission_denied" });
    expect(await createDisputeAction(CREATE_INPUT)).toEqual({
      ok: false,
      error: MSG.perm,
    });
  });

  // Scenario: Return or order not found
  it("maps NOT_FOUND to the not-found message", async () => {
    vi.mocked(createDispute).mockResolvedValue({ status: "not_found" });
    expect(await createDisputeAction(CREATE_INPUT)).toEqual({
      ok: false,
      error: MSG.notFound,
    });
  });

  // Scenario: Server refuses the defendant
  it("maps INVALID_ARGUMENT to the reload message", async () => {
    vi.mocked(createDispute).mockResolvedValue({ status: "invalid_argument" });
    expect(await createDisputeAction(CREATE_INPUT)).toEqual({
      ok: false,
      error: MSG.reload,
    });
  });

  // Scenario: Gateway outage on create
  it("maps outage and never forwards err.message", async () => {
    vi.mocked(createDispute).mockResolvedValue({ status: "unavailable" });
    expect(await createDisputeAction(CREATE_INPUT)).toEqual({
      ok: false,
      error: MSG.outage,
    });
    vi.mocked(createDispute).mockRejectedValue(new Error("secret-internal"));
    const res = await createDisputeAction(CREATE_INPUT);
    expect(res).toEqual({ ok: false, error: MSG.outage });
    expect(JSON.stringify(res)).not.toContain("secret-internal");
  });
});
