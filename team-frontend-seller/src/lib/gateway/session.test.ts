import { beforeEach, describe, expect, it, vi } from "vitest";

const cookieGet = vi.fn();
vi.mock("next/headers", () => ({ cookies: () => ({ get: cookieGet }) }));

import { getToken } from "./session.js";

function jwt(exp: number): string {
  const b = (o: object) =>
    btoa(JSON.stringify(o))
      .replace(/=+$/, "")
      .replace(/\+/g, "-")
      .replace(/\//g, "_");
  return `${b({ alg: "RS256" })}.${b({ sub: "u1", exp })}.sig`;
}
const nowSec = () => Math.floor(Date.now() / 1000);

beforeEach(() => cookieGet.mockReset());

describe("getToken", () => {
  it("returns a live session token", () => {
    const t = jwt(nowSec() + 3600);
    cookieGet.mockReturnValue({ value: t });
    expect(getToken()).toBe(t);
  });
  it("returns undefined (anonymous) for an expired token", () => {
    cookieGet.mockReturnValue({ value: jwt(nowSec() - 60) });
    expect(getToken()).toBeUndefined();
  });
  it("returns undefined for a malformed cookie", () => {
    cookieGet.mockReturnValue({ value: "not.a.jwt" });
    expect(getToken()).toBeUndefined();
  });
  it("returns undefined when there is no cookie", () => {
    cookieGet.mockReturnValue(undefined);
    expect(getToken()).toBeUndefined();
  });
});
