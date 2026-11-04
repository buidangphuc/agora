import { afterEach, describe, expect, it, vi } from "vitest";

import { GET, HEAD, POST } from "./route";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("auth handoff route handlers", () => {
  it("[Buyer to seller signed in / Seller to buyer signed in] mounts GET, HEAD and POST", () => {
    expect(typeof GET).toBe("function");
    expect(typeof HEAD).toBe("function");
    expect(typeof POST).toBe("function");
  });

  it("[Missing or oversized code] GET without a code redirects to /login without calling the gateway", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    const res = await GET(new Request("http://app.test/auth/handoff"));
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toMatch(/^\/login/);
    expect(res.headers.get("referrer-policy")).toBe("no-referrer");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("[Forged cross-site POST is refused] POST without Origin or Sec-Fetch-Site redirects to /login with no cookie", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    const res = await POST(
      new Request("http://app.test/auth/handoff", {
        method: "POST",
        body: new URLSearchParams({ code: "C", next: "/x" }),
      }),
    );
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe("/login");
    expect(res.headers.get("set-cookie")).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
