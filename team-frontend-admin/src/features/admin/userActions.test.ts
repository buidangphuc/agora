import { revalidatePath } from "next/cache";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getPrincipal } from "@/lib/gateway/session";
import { setUserStatus } from "@/lib/gateway/users";

import { setUserStatusAction } from "./userActions";

vi.mock("@/lib/gateway/users", () => ({
  listUsers: vi.fn(),
  setUserStatus: vi.fn(),
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

const SUCCESS = (n: number) =>
  `Đã cập nhật trạng thái; đã đăng xuất ${n} phiên.`;
const USER_VIEW = {
  id: "u-1",
  username: "buyer1",
  roles: ["buyer"],
  status: "active" as const,
  statusReason: "",
  statusUntil: "",
};

/** Form fields mirror the row action form: userId, status, reason, note, until. */
function form(fields: Record<string, string>): FormData {
  const fd = new FormData();
  const base = { userId: "u-1", status: "banned", reason: "abuse", note: "" };
  for (const [k, v] of Object.entries({ ...base, ...fields })) fd.set(k, v);
  return fd;
}

function okResult(sessionsRevoked = 0) {
  vi.mocked(setUserStatus).mockResolvedValue({
    status: "ok",
    user: USER_VIEW,
    sessionsRevoked,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  // 2026-10-17 03:00:00Z == 10:00 in UTC+07:00
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-17T03:00:00Z"));
  vi.mocked(getPrincipal).mockReturnValue({
    id: "admin-1",
    name: "root",
    scopes: ["admin"],
  });
});
afterEach(() => vi.useRealTimers());

describe("setUserStatusAction", () => {
  // Scenario: Users UI: Suspend with an end time
  it("suspend sends until in UTC+7", async () => {
    okResult(1);
    const res = await setUserStatusAction(
      form({
        status: "suspended",
        reason: "fraud",
        note: "bán hàng giả",
        until: "2026-10-20T17:00",
      }),
    );
    expect(res.ok).toBe(true);
    expect(setUserStatus).toHaveBeenCalledTimes(1);
    const arg = vi.mocked(setUserStatus).mock.calls[0][0];
    expect(arg.userId).toBe("u-1");
    expect(arg.status).toBe("suspended");
    expect(arg.reasonCode).toBe("fraud");
    expect(arg.note).toBe("bán hàng giả");
    expect(arg.until?.toISOString()).toBe("2026-10-20T10:00:00.000Z");
    expect(revalidatePath).toHaveBeenCalledTimes(1);
    expect(revalidatePath).toHaveBeenCalledWith("/admin/users");
  });

  // Scenario: Users UI: Ban and restrict send no end time
  it.each(["banned", "restricted"])(
    "until is sent only for suspended (%s)",
    async (status) => {
      okResult();
      const res = await setUserStatusAction(
        form({ status, reason: "abuse", until: "2026-10-20T17:00" }),
      );
      expect(res.ok).toBe(true);
      expect(setUserStatus).toHaveBeenCalledTimes(1);
      const arg = vi.mocked(setUserStatus).mock.calls[0][0];
      expect(arg.status).toBe(status);
      expect(arg.until).toBeUndefined();
    },
  );

  // Scenario: Users UI: Success shows how many sessions ended
  it("reports sessions revoked", async () => {
    okResult(2);
    const res = await setUserStatusAction(form({ status: "banned" }));
    expect(res.ok).toBe(true);
    expect(JSON.stringify(res)).toContain(SUCCESS(2));
  });

  // Scenario: Users UI: Reinstating shows zero sessions ended
  it("reinstating reports zero sessions", async () => {
    okResult(0);
    const res = await setUserStatusAction(
      form({ status: "active", reason: "other", until: "2026-10-20T17:00" }),
    );
    expect(res.ok).toBe(true);
    expect(JSON.stringify(res)).toContain(SUCCESS(0));
    expect(vi.mocked(setUserStatus).mock.calls[0][0].until).toBeUndefined();
  });

  // Scenario: Users UI: Invalid input or unknown user from the server is reported
  it("maps invalid and not found to fixed messages", async () => {
    vi.mocked(setUserStatus).mockResolvedValueOnce({ status: "invalid" });
    const a = await setUserStatusAction(form({}));
    vi.mocked(setUserStatus).mockResolvedValueOnce({ status: "not_found" });
    const b = await setUserStatusAction(form({}));
    expect(a).toEqual({
      ok: false,
      error: "Dữ liệu không hợp lệ; trạng thái không thay đổi.",
    });
    expect(b).toEqual({
      ok: false,
      error: "Không tìm thấy người dùng này; trạng thái không thay đổi.",
    });
    expect(JSON.stringify([a, b])).not.toContain("reason_code too long");
    expect(JSON.stringify([a, b])).not.toContain("phiên.");
  });

  // Scenario: Users UI: Missing reason is refused locally
  it.each([""])("refuses an empty reason (%j)", async (reason) => {
    const res = await setUserStatusAction(form({ reason }));
    expect(res).toEqual({ ok: false, error: "Vui lòng chọn mã lý do." });
    expect(setUserStatus).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Users UI: Suspension without a future end time is refused locally
  it.each([
    ["no end time", ""],
    // now is 10:00 UTC+7; 09:59 is one minute in the past
    ["one minute in the past", "2026-10-17T09:59"],
  ])("refuses missing or past until (%s)", async (_n, until) => {
    const res = await setUserStatusAction(
      form({ status: "suspended", reason: "fraud", until }),
    );
    expect(res).toEqual({
      ok: false,
      error: "Chọn thời điểm kết thúc tạm khóa trong tương lai.",
    });
    expect(setUserStatus).not.toHaveBeenCalled();
  });

  // Scenario: Users UI: Note of 501 characters is refused locally
  it("note boundary 500", async () => {
    const refused = await setUserStatusAction(form({ note: "a".repeat(501) }));
    expect(refused).toEqual({ ok: false, error: "Ghi chú tối đa 500 ký tự." });
    expect(setUserStatus).not.toHaveBeenCalled();

    okResult();
    const accepted = await setUserStatusAction(form({ note: "a".repeat(500) }));
    expect(accepted.ok).toBe(true);
    expect(setUserStatus).toHaveBeenCalledTimes(1);
    expect(vi.mocked(setUserStatus).mock.calls[0][0].note).toHaveLength(500);
  });

  // Scenario: Users UI: Non-admin cannot run the action
  it("rechecks admin and session", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "b-1",
      name: "buyer",
      scopes: ["listing.read"],
    });
    const res = await setUserStatusAction(form({}));
    expect(res.ok).toBe(false);
    expect(setUserStatus).not.toHaveBeenCalled();

    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(setUserStatusAction(form({}))).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/users");
    expect(setUserStatus).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  // Scenario: Users UI: Expired session during the action is sent to login
  it("redirects on unauthenticated", async () => {
    vi.mocked(setUserStatus).mockResolvedValue({ status: "unauthenticated" });
    await expect(setUserStatusAction(form({}))).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirectGuestToLogin).toHaveBeenCalledWith("/admin/users");
  });
});
