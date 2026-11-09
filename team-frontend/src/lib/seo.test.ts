import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const headersMock = vi.hoisted(() =>
  vi.fn(() => {
    throw new Error("request headers must not be read");
  }),
);
vi.mock("next/headers", () => ({
  headers: headersMock,
  cookies: headersMock,
}));

async function load() {
  vi.resetModules();
  return import("./seo");
}

beforeEach(() => {
  vi.unstubAllEnvs();
  Reflect.deleteProperty(process.env, "NEXT_PUBLIC_SITE_URL");
});

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("siteUrl (SEO-3)", () => {
  it("SEO-3 defaults to http://localhost:3000 when unset", async () => {
    const { siteUrl } = await load();
    expect(siteUrl()).toBe("http://localhost:3000");
  });

  it("SEO-3 removes the trailing slash", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://agora.example/");
    const { siteUrl } = await load();
    expect(siteUrl()).toBe("https://agora.example");
  });

  it("SEO-3 keeps an origin without a trailing slash unchanged", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://agora.example");
    const { siteUrl } = await load();
    expect(siteUrl()).toBe("https://agora.example");
  });

  it("SEO-3 never reads request headers", async () => {
    const { siteUrl, absoluteUrl } = await load();
    siteUrl();
    absoluteUrl("/listing/L1");
    expect(headersMock).not.toHaveBeenCalled();
  });
});

describe("absoluteUrl (SEO-3)", () => {
  it("SEO-3 joins a path onto the site origin", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://agora.example/");
    const { absoluteUrl } = await load();
    expect(absoluteUrl("/listing/L1")).toBe("https://agora.example/listing/L1");
    expect(absoluteUrl("/sitemap.xml")).toBe(
      "https://agora.example/sitemap.xml",
    );
  });

  it("SEO-3 keeps a query string on the path", async () => {
    const { absoluteUrl } = await load();
    expect(absoluteUrl("/search?category=c1")).toBe(
      "http://localhost:3000/search?category=c1",
    );
  });

  it("SEO-3 the root path ends with a single slash", async () => {
    const { absoluteUrl } = await load();
    expect(absoluteUrl("/")).toBe("http://localhost:3000/");
  });
});

describe("pageMetadata description (SEO-6)", () => {
  it("SEO-6 cuts a long multi-line description to at most 160 characters with no line break", async () => {
    const { pageMetadata } = await load();
    const long = Array.from({ length: 40 }, (_, i) => `dong ${i} noi dung`)
      .join("\n")
      .padEnd(400, "x");
    const md = pageMetadata({
      title: "Ao thun",
      description: long,
      path: "/listing/L1",
    });
    expect(typeof md.description).toBe("string");
    expect((md.description as string).length).toBeLessThanOrEqual(160);
    expect(md.description as string).not.toMatch(/[\r\n]/);
    const og = md.openGraph as { description?: string };
    expect(og.description?.length).toBeLessThanOrEqual(160);
    expect(og.description).not.toMatch(/[\r\n]/);
  });

  it("SEO-6 keeps a short description whole with whitespace collapsed", async () => {
    const { pageMetadata } = await load();
    const md = pageMetadata({
      title: "Ao thun",
      description: "Hai   dong\nmot  cau",
      path: "/listing/L1",
    });
    expect(md.description).toBe("Hai dong mot cau");
  });

  it("SEO-6 a description of exactly 160 characters is kept whole", async () => {
    const { pageMetadata } = await load();
    const exact = "a".repeat(160);
    const md = pageMetadata({
      title: "Ao thun",
      description: exact,
      path: "/listing/L1",
    });
    expect(md.description).toBe(exact);
  });

  it("SEO-3 canonical and og:url are absolute on the site origin, never on a request host", async () => {
    vi.stubEnv("NEXT_PUBLIC_SITE_URL", "https://agora.example/");
    const { pageMetadata } = await load();
    const md = pageMetadata({
      title: "Ao thun",
      description: "d",
      path: "/listing/L1",
    });
    expect(String(md.alternates?.canonical)).toBe(
      "https://agora.example/listing/L1",
    );
    expect(String(md.openGraph?.url)).toBe("https://agora.example/listing/L1");
    expect(headersMock).not.toHaveBeenCalled();
  });
});
