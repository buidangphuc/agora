import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { resolveShareLink } from "@/lib/gateway/sharing";

vi.mock("@/lib/gateway/sharing", () => ({ resolveShareLink: vi.fn() }));

import { GET } from "./route";

function link(
  targetType: string,
  targetId: string,
  utm: Record<string, string>,
) {
  vi.mocked(resolveShareLink).mockResolvedValue({
    targetType,
    targetId,
    utm,
    ogTitle: "",
    ogDescription: "",
    ogImageUrl: "",
    clickCount: 0,
  });
}

async function open(code = "sh0rt", headers: Record<string, string> = {}) {
  const res = await GET(
    new NextRequest(`http://localhost:3000/s/${code}`, { headers }),
    { params: Promise.resolve({ code }) },
  );
  const loc = new URL(
    res.headers.get("location") ?? "",
    "http://localhost:3000",
  );
  return { res, loc, path: `${loc.pathname}${loc.search}` };
}

function setCookies(res: Response): string[] {
  return res.headers.getSetCookie();
}

beforeEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(process.env, "COOKIE_SECURE");
});

describe("GET /s/[code]", () => {
  it("S5 referral link redirects to register with sorted utm and sets agora_ref", async () => {
    link("referral", "ABC123", {
      utm_source: "referral",
      utm_medium: "invite",
    });
    const { res, path, loc } = await open();
    expect(res.status).toBe(307);
    expect(loc.origin).toBe("http://localhost:3000");
    expect(path).toBe(
      "/register?ref=ABC123&utm_medium=invite&utm_source=referral",
    );
    const cookies = setCookies(res);
    expect(cookies).toHaveLength(1);
    const c = cookies[0];
    expect(c).toMatch(/^agora_ref=ABC123;/);
    expect(c).toMatch(/HttpOnly/i);
    expect(c).toMatch(/SameSite=lax/i);
    expect(c).toMatch(/Path=\//);
    expect(c).toMatch(/Max-Age=604800/);
    expect(c).not.toMatch(/Secure/i);
  });

  it("S5 marks the cookie Secure behind an https proxy", async () => {
    link("referral", "ABC123", {});
    const { res } = await open("sh0rt", { "x-forwarded-proto": "https" });
    expect(setCookies(res)[0]).toMatch(/Secure/i);
  });

  it("S5 marks the cookie Secure when COOKIE_SECURE=true", async () => {
    process.env.COOKIE_SECURE = "true";
    link("referral", "ABC123", {});
    const { res } = await open();
    expect(setCookies(res)[0]).toMatch(/Secure/i);
  });

  it("S5 redirects to /register?ref= alone when no utm is stored", async () => {
    link("referral", "ABC123", {});
    const { path } = await open();
    expect(path).toBe("/register?ref=ABC123");
  });

  it("S6 listing link keeps utm and sets no cookie", async () => {
    link("listing", "L42", { utm_source: "share" });
    const { res, path } = await open();
    expect(res.status).toBe(307);
    expect(path).toBe("/listing/L42?utm_source=share");
    expect(setCookies(res).join(";")).not.toContain("agora_ref");
  });

  it("S6 shop and seller links redirect to /shop/<id> with utm and no cookie", async () => {
    for (const type of ["shop", "seller"]) {
      link(type, "S7", { utm_source: "share", utm_campaign: "spring" });
      const { res, path } = await open();
      expect(res.status).toBe(307);
      expect(path).toBe("/shop/S7?utm_campaign=spring&utm_source=share");
      expect(setCookies(res).join(";")).not.toContain("agora_ref");
    }
  });

  it("S6 listing link without utm has a bare path", async () => {
    link("listing", "L42", {});
    const { path } = await open();
    expect(path).toBe("/listing/L42");
  });

  it("S7 redirects to / when the short code does not resolve", async () => {
    vi.mocked(resolveShareLink).mockResolvedValue(null);
    const { res, path } = await open("nope");
    expect(res.status).toBe(307);
    expect(path).toBe("/");
    expect(setCookies(res)).toEqual([]);
  });

  it("S7 redirects to / when resolving throws", async () => {
    vi.mocked(resolveShareLink).mockRejectedValue(new Error("down"));
    const { res, path } = await open("nope");
    expect(res.status).toBe(307);
    expect(path).toBe("/");
    expect(setCookies(res)).toEqual([]);
  });

  it("S8 unknown target type redirects to / with no utm and no cookie", async () => {
    link("order", "O1", { utm_source: "x" });
    const { res, path } = await open();
    expect(res.status).toBe(307);
    expect(path).toBe("/");
    expect(setCookies(res)).toEqual([]);
  });

  it("S9a drops non-utm keys and keeps utm_* keys", async () => {
    link("listing", "L42", {
      utm_source: "share",
      foo: "bar",
      UTM_x: "up",
      redirect: "https://evil.example",
    });
    const { path, loc } = await open();
    expect(path).toBe("/listing/L42?utm_source=share");
    expect(loc.searchParams.has("foo")).toBe(false);
    expect(loc.searchParams.has("redirect")).toBe(false);
  });

  it("S9a URL-encodes utm values so the Location stays on the app origin", async () => {
    link("listing", "L42", { utm_source: "a&b=c/../d" });
    const { loc } = await open();
    expect(loc.origin).toBe("http://localhost:3000");
    expect(loc.pathname).toBe("/listing/L42");
    expect(loc.searchParams.get("utm_source")).toBe("a&b=c/../d");
    expect([...loc.searchParams.keys()]).toEqual(["utm_source"]);
  });

  it.each([
    ["has a space", "bad code"],
    ["has CRLF", "AB\r\nSet-Cookie: x=1"],
    ["is 33 chars", "A".repeat(33)],
    ["is empty", ""],
    ["has a semicolon", "AB;Path=/"],
  ])(
    "S9b referral id that %s redirects to / with no cookie",
    async (_n, id) => {
      link("referral", id, { utm_source: "referral" });
      const { res, path } = await open();
      expect(res.status).toBe(307);
      expect(path).toBe("/");
      expect(setCookies(res)).toEqual([]);
    },
  );

  it("S9b accepts a 32-char id with dash and underscore, case kept", async () => {
    const id = `Ab_-${"x".repeat(28)}`;
    link("referral", id, {});
    const { res, path } = await open();
    expect(path).toBe(`/register?ref=${id}`);
    expect(setCookies(res)[0]).toMatch(new RegExp(`^agora_ref=${id};`));
  });

  it("S5 resolves the short code taken from the route params", async () => {
    link("listing", "L1", {});
    await open("Xy9");
    expect(resolveShareLink).toHaveBeenCalledWith("Xy9");
  });
});
