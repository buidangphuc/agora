import { beforeEach, describe, expect, it, vi } from "vitest";

import { makeClients } from "@/lib/gateway/client";

vi.mock("@/lib/gateway/client", () => ({
  makeClients: vi.fn(() => {
    throw new Error("gateway must not be called");
  }),
}));

import robots from "./robots";

const PREFIXES = [
  "/account",
  "/cart",
  "/checkout",
  "/seller",
  "/admin",
  "/login",
  "/register",
  "/chat",
  "/notifications",
  "/favorites",
  "/dev",
  "/api/",
];

function rules() {
  const r = robots().rules;
  return Array.isArray(r) ? r : [r];
}

beforeEach(() => {
  vi.clearAllMocks();
  Reflect.deleteProperty(process.env, "NEXT_PUBLIC_SITE_URL");
});

describe("robots", () => {
  it("SEO-22 robots makes no gateway call", () => {
    const first = robots();
    const second = robots();
    expect(makeClients).not.toHaveBeenCalled();
    expect(second).toEqual(first);
  });

  it("SEO-21 allows everything for every agent and disallows the twelve private prefixes", () => {
    const [rule] = rules();
    expect(rules()).toHaveLength(1);
    expect(rule?.userAgent).toBe("*");
    expect(rule?.allow).toBe("/");
    const disallow = ([] as string[]).concat(rule?.disallow ?? []);
    for (const p of PREFIXES) expect(disallow).toContain(p);
    expect(disallow).toHaveLength(PREFIXES.length);
  });

  it("SEO-30 disallows the seller and admin prefixes served by other apps", () => {
    const disallow = ([] as string[]).concat(rules()[0]?.disallow ?? []);
    expect(disallow).toContain("/seller");
    expect(disallow).toContain("/admin");
  });

  it("SEO-21 lists the absolute sitemap on the site origin", () => {
    expect(robots().sitemap).toBe("http://localhost:3000/sitemap.xml");
  });

  it("SEO-21 the sitemap line follows NEXT_PUBLIC_SITE_URL without a trailing slash", () => {
    process.env.NEXT_PUBLIC_SITE_URL = "https://agora.example/";
    expect(robots().sitemap).toBe("https://agora.example/sitemap.xml");
  });
});
