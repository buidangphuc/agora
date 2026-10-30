import { ApiError } from "@fe-platform/api-client";
import { beforeEach, describe, expect, it, vi } from "vitest";

const create = vi.fn();
const token = vi.fn();

vi.mock("@fe-platform/api-client", async (orig) => ({
  ...(await orig<typeof import("@fe-platform/api-client")>()),
  createGatewayClients: (...a: unknown[]) => create(...a),
}));
vi.mock("next/navigation", () => ({
  notFound: () => {
    throw new Error("NEXT_NOT_FOUND");
  },
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));
vi.mock("./session", () => ({
  requireSession: (next: string) => token(next),
  safeNext: (n: string) => n,
}));

import { attempt, errorMessage, guarded } from "./guard";

beforeEach(() => {
  create.mockReset();
  token.mockReset();
  token.mockResolvedValue("tok");
  create.mockReturnValue({ marker: true });
});

describe("guarded", () => {
  it("runs with authenticated clients", async () => {
    await expect(guarded("/x", async (c) => c)).resolves.toEqual({
      marker: true,
    });
    expect(create).toHaveBeenCalledWith({ token: "tok" });
    expect(token).toHaveBeenCalledWith("/x");
  });

  it("sends an unauthenticated failure to the refresh route", async () => {
    await expect(
      guarded("/shop/cart", async () => {
        throw new ApiError("unauthenticated", "no");
      }),
    ).rejects.toThrow("NEXT_REDIRECT:/session/refresh?next=%2Fshop%2Fcart");
  });

  it("turns not_found into notFound and rethrows others", async () => {
    await expect(
      guarded("/x", async () => {
        throw new ApiError("not_found", "no");
      }),
    ).rejects.toThrow("NEXT_NOT_FOUND");
    await expect(
      guarded("/x", async () => {
        throw new ApiError("unavailable", "down");
      }),
    ).rejects.toMatchObject({ kind: "unavailable" });
  });
});

describe("attempt", () => {
  it("returns the value or a failure code", async () => {
    await expect(attempt("/x", async () => 5)).resolves.toEqual({
      ok: true,
      value: 5,
    });
    await expect(
      attempt("/x", async () => {
        throw new ApiError("invalid", "bad");
      }),
    ).resolves.toEqual({ ok: false, code: "invalid" });
  });

  it("redirects an unauthenticated failure", async () => {
    await expect(
      attempt("/x", async () => {
        throw new ApiError("unauthenticated", "no");
      }),
    ).rejects.toThrow("NEXT_REDIRECT:/session/refresh?next=%2Fx");
  });
});

describe("errorMessage", () => {
  it("maps known codes and falls back", () => {
    expect(errorMessage(undefined)).toBeUndefined();
    expect(errorMessage("invalid")).toContain("not accepted");
    expect(errorMessage(["unavailable"])).toContain("busy");
    expect(errorMessage("weird")).toContain("Something went wrong");
  });
});
