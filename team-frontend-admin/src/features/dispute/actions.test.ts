import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import {
  type ViewDispute,
  createDispute,
  resolveDispute,
} from "@/lib/gateway/disputes";
import { getPrincipal } from "@/lib/gateway/session";

import { resolveDisputeAction } from "./actions";

vi.mock("@/lib/gateway/disputes", () => ({
  createDispute: vi.fn(),
  resolveDispute: vi.fn(),
  listDisputes: vi.fn(),
  listOrderDisputes: vi.fn(),
  getDispute: vi.fn(),
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
//   resolveDisputeAction({ disputeId, status, resolution, ruling? })
//     => ActionResult<ViewDispute>; admin scope re-checked via getPrincipal().
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
  vi.mocked(resolveDispute).mockResolvedValue({
    status: "ok",
    dispute: { ...dispute, status: "INVESTIGATING" },
  });
});

describe("resolveDisputeAction", () => {
  // Scenario: Start investigating
  it("resolveDisputeAction moves OPEN to INVESTIGATING", async () => {
    const res = await resolveDisputeAction({
      disputeId: "d1",
      status: "INVESTIGATING",
      resolution: "",
    });
    expect(res).toEqual({
      ok: true,
      data: { ...dispute, status: "INVESTIGATING" },
    });
    expect(resolveDispute).toHaveBeenCalledTimes(1);
    const arg = vi.mocked(resolveDispute).mock.calls[0][0];
    expect(arg.disputeId).toBe("d1");
    expect(arg.status).toBe("INVESTIGATING");
    expect(arg.ruling ?? "").toBe("");
    expect(vi.mocked(revalidatePath)).toHaveBeenCalledWith("/admin/cases");
  });

  // Scenario: Seller-favour and reject send the right payload
  it("sends seller-favour and reject payloads", async () => {
    await resolveDisputeAction({
      disputeId: "d1",
      status: "RESOLVED",
      resolution: "Hàng đúng mô tả",
      ruling: "SELLER_FAVOUR",
    });
    await resolveDisputeAction({
      disputeId: "d2",
      status: "REJECTED",
      resolution: "Spam",
    });
    const calls = vi.mocked(resolveDispute).mock.calls;
    expect(calls).toHaveLength(2);
    expect(calls[0][0]).toMatchObject({
      disputeId: "d1",
      status: "RESOLVED",
      resolution: "Hàng đúng mô tả",
      ruling: "SELLER_FAVOUR",
    });
    expect(calls[1][0]).toMatchObject({
      disputeId: "d2",
      status: "REJECTED",
      resolution: "Spam",
    });
    expect(calls[1][0].ruling ?? "").toBe("");
  });

  // Scenario: Server refuses the transition
  it("maps a refused transition", async () => {
    for (const s of ["failed_precondition", "invalid_argument"] as const) {
      vi.mocked(resolveDispute).mockResolvedValue({ status: s });
      expect(
        await resolveDisputeAction({
          disputeId: "d1",
          status: "RESOLVED",
          resolution: "x",
          ruling: "BUYER_FAVOUR",
        }),
      ).toEqual({ ok: false, error: MSG.transition });
    }
  });

  // Scenario: Audit service is down
  it("maps UNAVAILABLE to the audit-down message", async () => {
    vi.mocked(resolveDispute).mockResolvedValue({ status: "unavailable" });
    expect(
      await resolveDisputeAction({
        disputeId: "d1",
        status: "INVESTIGATING",
        resolution: "",
      }),
    ).toEqual({ ok: false, error: MSG.audit });
  });

  // Scenario: Resolve times out
  it("maps a resolve timeout to the unknown-state message", async () => {
    vi.mocked(resolveDispute).mockResolvedValue({ status: "timeout" });
    const res = await resolveDisputeAction({
      disputeId: "d1",
      status: "INVESTIGATING",
      resolution: "",
    });
    expect(res).toEqual({ ok: false, error: MSG.timeout });
    expect(res).not.toEqual({ ok: false, error: MSG.audit });
    expect(vi.mocked(revalidatePath)).toHaveBeenCalledWith("/admin/cases");
  });

  // Scenario: Non-admin cannot call the action
  it("refuses a non-admin before calling the gateway", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u1",
      name: "buyer",
      scopes: ["user"],
    });
    const res = await resolveDisputeAction({
      disputeId: "d1",
      status: "INVESTIGATING",
      resolution: "",
    });
    expect(res.ok).toBe(false);
    expect(resolveDispute).not.toHaveBeenCalled();
  });

  // Scenario: Non-admin cannot call the action
  it("redirects a guest to login without calling the gateway", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(
      resolveDisputeAction({
        disputeId: "d1",
        status: "INVESTIGATING",
        resolution: "",
      }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirectGuestToLogin).toHaveBeenCalled();
    expect(resolveDispute).not.toHaveBeenCalled();
  });
});
