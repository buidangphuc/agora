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

import { startBuyerHandoff } from "./actions";

beforeEach(() => vi.clearAllMocks());

describe("seller startBuyerHandoff action", () => {
  it("[Seller to buyer signed in] starts a seller-session hand-off towards the buyer target with the next path", async () => {
    startHandoff.mockResolvedValue("http://localhost:3000/auth/handoff?code=C");
    const url = await startBuyerHandoff("/chat");
    expect(url).toBe("http://localhost:3000/auth/handoff?code=C");
    const [cfg, target, next] = startHandoff.mock.calls[0];
    expect(cfg.app).toBe("seller");
    expect(target).toBe("buyer");
    expect(next).toBe("/chat");
  });

  it("[Mint failure still navigates] returns the buyer login URL when startHandoff throws", async () => {
    startHandoff.mockRejectedValue(new Error("boom"));
    await expect(startBuyerHandoff("/chat")).resolves.toBe(
      "http://localhost:3000/login?next=%2Fchat",
    );
  });
});
