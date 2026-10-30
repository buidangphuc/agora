import { ApiError } from "@fe-platform/api-client";
import { beforeEach, describe, expect, it, vi } from "vitest";

const auth = { login: vi.fn(), register: vi.fn() };
const session = { setSession: vi.fn(), clearSession: vi.fn() };

vi.mock("@fe-platform/api-client", async (orig) => ({
  ...(await orig<typeof import("@fe-platform/api-client")>()),
  createGatewayClients: () => ({ auth }),
}));
vi.mock("next/navigation", () => ({
  redirect: (to: string) => {
    throw new Error(`NEXT_REDIRECT:${to}`);
  },
}));
vi.mock("../../../lib/session", () => ({
  safeNext: (n?: string) =>
    n?.startsWith("/") && !n.startsWith("//") ? n : "/",
  setSession: (...a: unknown[]) => session.setSession(...a),
  clearSession: () => session.clearSession(),
}));

import { authErrorCode } from "../../../lib/auth-errors";
import { register } from "../register/actions";
import { login, logout } from "./actions";

const form = (o: Record<string, string>) => {
  const f = new FormData();
  for (const [k, v] of Object.entries(o)) f.set(k, v);
  return f;
};
const ok = {
  result: { token: "t", refreshToken: "r", refreshExpiresAt: undefined },
};

beforeEach(() => {
  auth.login.mockReset();
  auth.register.mockReset();
  session.setSession.mockReset();
  session.clearSession.mockReset();
});

describe("login", () => {
  it("stores the session and goes to a safe next", async () => {
    auth.login.mockResolvedValue(ok);
    await expect(
      login(form({ username: " an ", password: "pw", next: "/shop/cart" })),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/cart");
    expect(auth.login).toHaveBeenCalledWith({ username: "an", password: "pw" });
    expect(session.setSession).toHaveBeenCalledWith(
      expect.objectContaining({ token: "t", refreshToken: "r" }),
    );
  });

  it("ignores an external next", async () => {
    auth.login.mockResolvedValue(ok);
    await expect(
      login(
        form({ username: "an", password: "pw", next: "https://evil.test" }),
      ),
    ).rejects.toThrow("NEXT_REDIRECT:/");
  });

  it("reports credentials, blocked and unavailable without leaking the message", async () => {
    auth.login.mockRejectedValue(
      new ApiError("unauthenticated", "bad password for an"),
    );
    await expect(
      login(form({ username: "an", password: "x" })),
    ).rejects.toThrow("NEXT_REDIRECT:/login?error=credentials&next=%2F");
    auth.login.mockRejectedValue(new ApiError("forbidden", "suspended"));
    await expect(
      login(form({ username: "an", password: "x" })),
    ).rejects.toThrow("error=blocked");
    auth.login.mockRejectedValue(new ApiError("unavailable", "down"));
    await expect(
      login(form({ username: "an", password: "x" })),
    ).rejects.toThrow("error=unavailable");
    expect(session.setSession).not.toHaveBeenCalled();
  });

  it("treats an empty token as unavailable", async () => {
    auth.login.mockResolvedValue({ result: { token: "", refreshToken: "" } });
    await expect(
      login(form({ username: "an", password: "x" })),
    ).rejects.toThrow("error=unavailable");
  });
});

describe("register and logout", () => {
  it("registers, stores the session and redirects", async () => {
    auth.register.mockResolvedValue(ok);
    await expect(
      register(form({ username: "an", password: "pw", next: "/shop/cart" })),
    ).rejects.toThrow("NEXT_REDIRECT:/shop/cart");
    expect(auth.register).toHaveBeenCalledWith({
      username: "an",
      password: "pw",
    });
    expect(session.setSession).toHaveBeenCalled();
  });

  it("reports a refused registration", async () => {
    auth.register.mockRejectedValue(new ApiError("invalid", "username taken"));
    await expect(
      register(form({ username: "an", password: "pw" })),
    ).rejects.toThrow("NEXT_REDIRECT:/register?error=taken&next=%2F");
  });

  it("logout clears the session and goes home", async () => {
    await expect(logout()).rejects.toThrow("NEXT_REDIRECT:/");
    expect(session.clearSession).toHaveBeenCalled();
  });

  it("maps error kinds to codes", () => {
    expect(authErrorCode("forbidden")).toBe("blocked");
    expect(authErrorCode("internal")).toBe("unavailable");
    expect(authErrorCode("invalid")).toBe("credentials");
  });
});
