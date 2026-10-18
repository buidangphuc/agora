import { beforeEach, describe, expect, it, vi } from "vitest";

const headerGet = vi.hoisted(() => vi.fn());
const redirectToLogin = vi.hoisted(() => vi.fn());

vi.mock("next/headers", () => ({
  headers: async () => ({ get: headerGet }),
}));
vi.mock("./returnUrl", () => ({ redirectToLogin }));

import {
  buildReturnPath,
  isAdminPath,
  redirectGuestToLoginFromHeader,
} from "./guardPath";

beforeEach(() => {
  headerGet.mockReset();
  redirectToLogin.mockReset();
  redirectToLogin.mockImplementation(() => {
    throw new Error("NEXT_REDIRECT");
  });
});

describe("Referrer hygiene: buildReturnPath drops admin searchParams", () => {
  it("admin returnUrl is path only", () => {
    expect(
      buildReturnPath("/admin/audit", { target_id: "u-1", actor_id: ["a", "b"] }),
    ).toBe("/admin/audit");
  });

  it("admin root is path only", () => {
    expect(buildReturnPath("/admin", { x: "1" })).toBe("/admin");
  });
});

describe("Referrer hygiene: non-admin guard still preserves the query", () => {
  it("non-admin query preserved", () => {
    expect(buildReturnPath("/chat", { thread: "t1", tag: ["a", "b"] })).toBe(
      "/chat?thread=t1&tag=a&tag=b",
    );
  });
});

describe("Referrer hygiene: look-alike admin prefix keeps its query", () => {
  it("admin match is segment exact", () => {
    expect(buildReturnPath("/administrator", { q: "1" })).toBe(
      "/administrator?q=1",
    );
    expect(buildReturnPath("/adminx", { q: "1" })).toBe("/adminx?q=1");
  });

  it("isAdminPath is segment exact", () => {
    expect(isAdminPath("/admin")).toBe(true);
    expect(isAdminPath("/admin/users")).toBe(true);
    expect(isAdminPath("/administrator")).toBe(false);
    expect(isAdminPath("/adminx/users")).toBe(false);
    expect(isAdminPath("/chat")).toBe(false);
  });
});

describe("Referrer hygiene: header-based guard drops the admin query", () => {
  it("header guard strips admin query", async () => {
    headerGet.mockImplementation((n: string) =>
      n === "x-pathname" ? "/admin/cases?status=open" : null,
    );
    await expect(redirectGuestToLoginFromHeader()).rejects.toThrow();
    expect(redirectToLogin).toHaveBeenCalledWith("/admin/cases");
  });

  it("header guard keeps non-admin query", async () => {
    headerGet.mockImplementation((n: string) =>
      n === "x-pathname" ? "/chat?thread=t1" : null,
    );
    await expect(redirectGuestToLoginFromHeader()).rejects.toThrow();
    expect(redirectToLogin).toHaveBeenCalledWith("/chat?thread=t1");
  });
});
