import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  cancelDeletion,
  exportMyData,
  recordConsents,
  requestDeletion,
} from "@/lib/gateway/privacy";

import {
  cancelDeletionAction,
  exportDataAction,
  requestDeletionAction,
  saveConsentAction,
} from "./actions";

vi.mock("@/lib/gateway/privacy", () => ({
  recordConsents: vi.fn(),
  requestDeletion: vi.fn(),
  cancelDeletion: vi.fn(),
  exportMyData: vi.fn(),
}));

const SESSION_EXPIRED = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";
const CONSENT_FAIL = "Chưa lưu được lựa chọn. Vui lòng thử lại.";
const EXPORT_FAIL = "Chưa tải được dữ liệu. Vui lòng thử lại sau.";

const consoleSpies = (["log", "info", "warn", "error", "debug"] as const).map(
  (m) => vi.spyOn(console, m).mockImplementation(() => undefined),
);
const logged = () => JSON.stringify(consoleSpies.flatMap((s) => s.mock.calls));

beforeEach(() => {
  vi.clearAllMocks();
});
afterEach(() => {
  vi.mocked(recordConsents).mockReset();
});

describe("saveConsentAction", () => {
  it("[PV5 Saving a change records one row per changed purpose] records only the changed purposes with source account", async () => {
    vi.mocked(recordConsents).mockResolvedValue(undefined);
    const res = await saveConsentAction([
      { purpose: "ANALYTICS", granted: false },
      { purpose: "MARKETING", granted: true },
    ]);
    expect(res).toEqual({ ok: true });
    expect(recordConsents).toHaveBeenCalledTimes(1);
    expect(vi.mocked(recordConsents).mock.calls[0][0]).toEqual([
      { purpose: "ANALYTICS", granted: false, version: "1", source: "account" },
      { purpose: "MARKETING", granted: true, version: "1", source: "account" },
    ]);
    expect(revalidatePath).toHaveBeenCalledWith("/account/privacy");
  });

  it("[PV10 An expired session on save asks to sign in again] maps Unauthenticated on consent save", async () => {
    vi.mocked(recordConsents).mockRejectedValue(
      new ConnectError("jwt expired", Code.Unauthenticated),
    );
    await expect(
      saveConsentAction([{ purpose: "ANALYTICS", granted: true }]),
    ).resolves.toEqual({ ok: false, error: SESSION_EXPIRED });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("[PV28 Unknown failures hide detail] maps InvalidArgument on consent to the fixed text", async () => {
    vi.mocked(recordConsents).mockRejectedValue(
      new ConnectError("bad purpose 9", Code.InvalidArgument),
    );
    await expect(
      saveConsentAction([{ purpose: "ANALYTICS", granted: true }]),
    ).resolves.toEqual({ ok: false, error: "Lựa chọn không hợp lệ." });
  });

  it("[PV28 Unknown failures hide detail] unmapped errors show only the fixed text", async () => {
    vi.mocked(recordConsents).mockRejectedValue(
      new ConnectError("db password=hunter2", Code.Internal),
    );
    await expect(
      saveConsentAction([{ purpose: "ANALYTICS", granted: true }]),
    ).resolves.toEqual({ ok: false, error: CONSENT_FAIL });
    vi.mocked(recordConsents).mockRejectedValue(new Error("stack trace here"));
    await expect(
      saveConsentAction([{ purpose: "ANALYTICS", granted: true }]),
    ).resolves.toEqual({ ok: false, error: CONSENT_FAIL });
  });
});

describe("exportDataAction", () => {
  it("[PV11 Export downloads one JSON file] export returns filename and content from the response", async () => {
    const content = '{"account":{"user_id":"u-1"}}';
    vi.mocked(exportMyData).mockResolvedValue({
      content,
      contentType: "application/json",
      generatedAt: "2026-11-14T23:30:00.000Z",
    });
    const res = await exportDataAction();
    expect(res).toEqual({
      ok: true,
      data: { filename: "agora-du-lieu-2026-11-14.json", content },
    });
  });

  it("[PV13 Export content is never retained or logged] export leaves no trace outside the response", async () => {
    const content = '{"email":"secret-value@example.com"}';
    vi.mocked(exportMyData).mockResolvedValue({
      content,
      contentType: "application/json",
      generatedAt: "2026-11-14T10:00:00.000Z",
    });
    await exportDataAction();
    expect(revalidatePath).not.toHaveBeenCalled();
    expect(logged()).not.toContain("secret-value");
  });

  it("[PV28 Unknown failures hide detail] unmapped errors show only the fixed text", async () => {
    vi.mocked(exportMyData).mockRejectedValue(
      new ConnectError("upstream 10.0.0.3 refused", Code.Unavailable),
    );
    await expect(exportDataAction()).resolves.toEqual({
      ok: false,
      error: EXPORT_FAIL,
    });
    vi.mocked(exportMyData).mockRejectedValue(new Error("boom detail"));
    await expect(exportDataAction()).resolves.toEqual({
      ok: false,
      error: EXPORT_FAIL,
    });
  });
});

describe("requestDeletionAction", () => {
  it("[PV16 Confirming with the password schedules deletion] request deletion sends only the password", async () => {
    vi.mocked(requestDeletion).mockResolvedValue({
      eraseAfter: "2026-11-16T10:00:00.000Z",
    });
    const res = await requestDeletionAction("s3cret-pw");
    expect(res.ok).toBe(true);
    expect(requestDeletion).toHaveBeenCalledTimes(1);
    expect(requestDeletion).toHaveBeenCalledWith("s3cret-pw");
    expect(revalidatePath).toHaveBeenCalledWith("/account/privacy");
  });

  it("[PV19 An open obligation shows the reason] maps FailedPrecondition to prefix plus category", async () => {
    vi.mocked(requestDeletion).mockRejectedValue(
      new ConnectError("còn đơn hàng đang mở", Code.FailedPrecondition),
    );
    await expect(requestDeletionAction("pw")).resolves.toEqual({
      ok: false,
      error: "Chưa thể xóa tài khoản: còn đơn hàng đang mở",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("[PV20 Obligation service outage fails closed] maps Unavailable on request deletion", async () => {
    vi.mocked(requestDeletion).mockRejectedValue(
      new ConnectError("commerce down", Code.Unavailable),
    );
    await expect(requestDeletionAction("pw")).resolves.toEqual({
      ok: false,
      error:
        "Chưa kiểm tra được đơn hàng và số dư của bạn. Vui lòng thử lại sau.",
    });
  });

  it("[PV17 Wrong password schedules nothing] maps PermissionDenied to the wrong password text", async () => {
    vi.mocked(requestDeletion).mockRejectedValue(
      new ConnectError("bad credentials", Code.PermissionDenied),
    );
    await expect(requestDeletionAction("pw")).resolves.toEqual({
      ok: false,
      error: "Mật khẩu không đúng.",
    });
  });

  it("[PV18 Empty password is blocked locally] maps InvalidArgument to the password prompt", async () => {
    vi.mocked(requestDeletion).mockRejectedValue(
      new ConnectError("password required", Code.InvalidArgument),
    );
    await expect(requestDeletionAction("")).resolves.toEqual({
      ok: false,
      error: "Vui lòng nhập mật khẩu.",
    });
  });

  it("[PV24 The password never leaves the action] never returns or logs the password", async () => {
    const pw = "Zx9-unique-password-value";
    vi.mocked(requestDeletion).mockResolvedValueOnce({
      eraseAfter: "2026-11-16T10:00:00.000Z",
    });
    const okRes = await requestDeletionAction(pw);
    vi.mocked(requestDeletion).mockRejectedValueOnce(
      new ConnectError(`bad ${pw}`, Code.Internal),
    );
    const failRes = await requestDeletionAction(pw);
    vi.mocked(requestDeletion).mockRejectedValueOnce(new Error(pw));
    const failRes2 = await requestDeletionAction(pw);
    expect(JSON.stringify([okRes, failRes, failRes2])).not.toContain(pw);
    expect(logged()).not.toContain(pw);
    expect(JSON.stringify(vi.mocked(revalidatePath).mock.calls)).not.toContain(
      pw,
    );
  });

  it("[PV28 Unknown failures hide detail] unmapped errors show only the fixed text", async () => {
    vi.mocked(requestDeletion).mockRejectedValue(
      new ConnectError("secret internal detail", Code.Internal),
    );
    const a = await requestDeletionAction("pw");
    vi.mocked(requestDeletion).mockRejectedValue(new Error("secret internal"));
    const b = await requestDeletionAction("pw");
    for (const r of [a, b]) {
      expect(r.ok).toBe(false);
      if (!r.ok) {
        expect(r.error.length).toBeGreaterThan(0);
        expect(r.error).not.toContain("secret internal");
      }
    }
  });
});

describe("cancelDeletionAction", () => {
  it("[PV21 Pending deletion can be cancelled] cancels once and revalidates the page", async () => {
    vi.mocked(cancelDeletion).mockResolvedValue(undefined);
    await expect(cancelDeletionAction()).resolves.toEqual({ ok: true });
    expect(cancelDeletion).toHaveBeenCalledTimes(1);
    expect(revalidatePath).toHaveBeenCalledWith("/account/privacy");
  });

  it("[PV22 Cancel with nothing pending is explained] maps FailedPrecondition on cancel and re-reads the status", async () => {
    vi.mocked(cancelDeletion).mockRejectedValue(
      new ConnectError("not pending", Code.FailedPrecondition),
    );
    await expect(cancelDeletionAction()).resolves.toEqual({
      ok: false,
      error: "Không có yêu cầu xóa tài khoản đang chờ.",
    });
    expect(revalidatePath).toHaveBeenCalledWith("/account/privacy");
  });

  it("[PV28 Unknown failures hide detail] unmapped errors show only the fixed text", async () => {
    vi.mocked(cancelDeletion).mockRejectedValue(
      new ConnectError("secret internal detail", Code.Internal),
    );
    const r = await cancelDeletionAction();
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.error).not.toContain("secret internal");
  });
});

describe("every action", () => {
  const cases: [string, () => Promise<{ ok: boolean; error?: string }>][] = [
    ["save", () => saveConsentAction([{ purpose: "MARKETING", granted: true }])],
    ["export", () => exportDataAction()],
    ["request", () => requestDeletionAction("pw")],
    ["cancel", () => cancelDeletionAction()],
  ];
  const rejectAll = (e: unknown) => {
    vi.mocked(recordConsents).mockRejectedValue(e);
    vi.mocked(exportMyData).mockRejectedValue(e);
    vi.mocked(requestDeletion).mockRejectedValue(e);
    vi.mocked(cancelDeletion).mockRejectedValue(e);
  };

  it.each(cases)(
    "[PV26 A call without a session is refused] maps Unauthenticated on every action (%s)",
    async (_n, run) => {
      rejectAll(new ConnectError("jwt expired", Code.Unauthenticated));
      await expect(run()).resolves.toEqual({
        ok: false,
        error: SESSION_EXPIRED,
      });
    },
  );

  it("[PV30 An unauthenticated export is refused] maps PermissionDenied on export to the session text", async () => {
    vi.mocked(exportMyData).mockRejectedValue(
      new ConnectError("denied", Code.PermissionDenied),
    );
    await expect(exportDataAction()).resolves.toEqual({
      ok: false,
      error: SESSION_EXPIRED,
    });
  });

  it("[PV31 Deletion by a session without rights is refused] maps Unauthenticated on request deletion", async () => {
    vi.mocked(requestDeletion).mockRejectedValue(
      new ConnectError("no session", Code.Unauthenticated),
    );
    await expect(requestDeletionAction("pw")).resolves.toEqual({
      ok: false,
      error: SESSION_EXPIRED,
    });
  });

  it("[PV29 A guest cannot reach any privacy data] returns no data when refused", async () => {
    rejectAll(new ConnectError("no session", Code.Unauthenticated));
    const r = await exportDataAction();
    expect(r).not.toHaveProperty("data");
  });
});
