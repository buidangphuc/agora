import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { revokeSession } from "@/lib/gateway/sessions";

import { revokeSessionAction } from "./actions";

vi.mock("@/lib/gateway/sessions", () => ({ revokeSession: vi.fn() }));

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
