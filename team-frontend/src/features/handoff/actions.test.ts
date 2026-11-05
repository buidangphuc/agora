import { beforeEach, describe, expect, it, vi } from "vitest";

const startHandoff = vi.hoisted(() => vi.fn());
vi.mock("@fe-platform/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@fe-platform/session")>()),
  startHandoff,
}));
vi.mock("next/headers", () => ({
  cookies: vi.fn(async () => ({ get: () => undefined })),
  headers: vi.fn(async () => new Headers()),
}));

import { startSellerHandoff } from "./actions";

const LOGIN = "http://localhost:3001/login?next=%2Fseller%2Forders";

beforeEach(() => vi.clearAllMocks());

describe("buyer startSellerHandoff action", () => {
  it("[Buyer to seller signed in] starts a buyer-session hand-off towards the seller target with the next path", async () => {
    startHandoff.mockResolvedValue("http://localhost:3001/auth/handoff?code=C");
    const url = await startSellerHandoff("/seller/orders");
    expect(url).toBe("http://localhost:3001/auth/handoff?code=C");
    expect(startHandoff).toHaveBeenCalledTimes(1);
    const [cfg, target, next] = startHandoff.mock.calls[0];
    expect(cfg.app).toBe("buyer");
    expect(target).toBe("seller");
    expect(next).toBe("/seller/orders");
  });

  it("[Mint failure still navigates] returns the seller login URL when startHandoff throws", async () => {
    startHandoff.mockRejectedValue(new Error("boom"));
    await expect(startSellerHandoff("/seller/orders")).resolves.toBe(LOGIN);
  });
});
