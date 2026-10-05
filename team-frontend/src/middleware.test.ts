// @vitest-environment node
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";

import { middleware } from "./middleware";

function jwt(exp: number): string {
  const b = (o: object) =>
    btoa(JSON.stringify(o))
      .replace(/=+$/, "")
      .replace(/\+/g, "-")
      .replace(/\//g, "_");
  return `${b({ alg: "RS256" })}.${b({ sub: "u1", exp })}.sig`;
}
const nowSec = () => Math.floor(Date.now() / 1000);

function req(cookie?: string) {
  return new NextRequest("http://localhost/search", {
    headers: cookie ? { cookie: `session=${cookie}` } : {},
  });
}

describe("session cookie middleware", () => {
  it("clears an expired session cookie and hides it from the render", () => {
    const res = middleware(req(jwt(nowSec() - 60)));
    expect(res.cookies.get("session")?.value).toBe("");
    // Next signals header overrides via x-middleware-override-headers; the
    // forwarded cookie header must no longer carry the session.
    expect(res.headers.get("x-middleware-request-cookie") ?? "").not.toContain(
      "session=",
    );
  });
  it("clears a malformed session cookie", () => {
    const res = middleware(req("not.a.jwt"));
    expect(res.cookies.get("session")?.value).toBe("");
  });
  it("leaves a live session untouched", () => {
    const res = middleware(req(jwt(nowSec() + 3600)));
    expect(res.cookies.get("session")).toBeUndefined();
  });
  it("leaves anonymous requests untouched", () => {
    const res = middleware(req());
    expect(res.cookies.get("session")).toBeUndefined();
  });
});
