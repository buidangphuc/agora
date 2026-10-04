import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { revokeSession } from "@/lib/gateway/sessions";
import { submitKyc } from "@/lib/gateway/verification";

import { revokeSessionAction } from "./actions";
import { submitKycAction } from "./verification/actions";

vi.mock("@/lib/gateway/sessions", () => ({ revokeSession: vi.fn() }));
vi.mock("@/lib/gateway/verification", () => ({ submitKyc: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("revokeSessionAction", () => {
  it("revokes, revalidates the security page and returns ok", async () => {
    vi.mocked(revokeSession).mockResolvedValue(undefined);
    await expect(revokeSessionAction("s1")).resolves.toEqual({ ok: true });
    expect(revokeSession).toHaveBeenCalledWith("s1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/security");
  });

  it("resolves with { ok: false, error } when the gateway throws", async () => {
    vi.mocked(revokeSession).mockRejectedValue(new Error("gateway down"));
    await expect(revokeSessionAction("s1")).resolves.toEqual({
      ok: false,
      error: "gateway down",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("submitKycAction", () => {
  it("submits, revalidates the verification page and returns ok", async () => {
    vi.mocked(submitKyc).mockResolvedValue({ id: "k", status: 1 } as never);
    await expect(submitKycAction("passport", "X123")).resolves.toEqual({
      ok: true,
    });
    expect(submitKyc).toHaveBeenCalledWith("passport", "X123");
    expect(revalidatePath).toHaveBeenCalledWith("/account/verification");
  });

  it("rejects an empty reference without calling the gateway", async () => {
    const res = await submitKycAction("passport", "  ");
    expect(res.ok).toBe(false);
    expect(submitKyc).not.toHaveBeenCalled();
  });

  it("resolves with { ok: false, error } when the gateway throws", async () => {
    vi.mocked(submitKyc).mockRejectedValue(new Error("rejected"));
    await expect(submitKycAction("passport", "X1")).resolves.toEqual({
      ok: false,
      error: "rejected",
    });
  });
});
