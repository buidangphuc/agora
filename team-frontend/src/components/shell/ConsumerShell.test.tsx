import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// ConsumerShell is an async server component with an async child (AuthNav), which jsdom cannot
// render, and jsdom has no layout; assert where the buy-bar space is reserved instead.
describe("buy-bar space", () => {
  const shell = readFileSync(join(__dirname, "ConsumerShell.tsx"), "utf8");
  const css = readFileSync(join(__dirname, "../../app/globals.css"), "utf8");

  it("is reserved on body below lg, not inside the footer", () => {
    expect(css).toMatch(
      /@media \(max-width: 1023\.98px\)\s*\{\s*body:has\(\[data-testid="buy-bar"\]\)\s*\{\s*padding-bottom: theme\("spacing\.24"\);/,
    );
    const footer = shell.slice(
      shell.indexOf("<footer"),
      shell.indexOf(">", shell.indexOf("<footer")),
    );
    expect(footer).not.toContain("buy-bar");
  });
});

describe("seller channel links", () => {
  const shell = readFileSync(join(__dirname, "ConsumerShell.tsx"), "utf8");

  it("[Buyer to seller signed in] go through SellerChannelLink, never a bare seller URL or href", () => {
    expect(shell.match(/<SellerChannelLink/g)).toHaveLength(2);
    expect(shell).not.toContain("sellerEntryUrl(");
    expect(shell).not.toContain("sellerAppUrl(");
  });
});

describe("footer legal links", () => {
  const shell = readFileSync(join(__dirname, "ConsumerShell.tsx"), "utf8");
  const footer = shell.slice(shell.indexOf("<footer"));
  const slugs = [
    "dieu-khoan-su-dung",
    "chinh-sach-bao-mat",
    "quy-che-hoat-dong",
    "chinh-sach-doi-tra",
    "hang-cam-kinh-doanh",
    "giai-quyet-khieu-nai",
    "dieu-khoan-nguoi-ban",
  ];

  it.each(slugs)("[Footer exposes the legal links] links /%s", (slug) => {
    expect(footer).toContain(`href="/${slug}"`);
  });

  it("[Footer exposes the legal links] RMA entry targets /chinh-sach-doi-tra, not /account/orders", () => {
    const m = footer.match(
      /<Link\s+href="([^"]+)"[^>]*>\s*Chính Sách Trả Hàng & Hoàn Tiền \(RMA\)/,
    );
    expect(m?.[1]).toBe("/chinh-sach-doi-tra");
  });

  it("[Footer shows the operator placeholders] renders the operator fields from operator.ts", () => {
    expect(shell).toMatch(/features\/legal\/operator/);
    expect(footer).toContain("OPERATOR_FIELDS");
  });

  it("[Footer never links legal pages that are missing] every footer legal href has a content file", () => {
    const hrefs = [...footer.matchAll(/href="\/([a-z-]+)"/g)].map((m) => m[1]);
    const legal = hrefs.filter((h) => slugs.includes(h));
    expect(new Set(legal).size).toBe(7);
    for (const h of new Set(legal)) {
      expect(existsSync(join(__dirname, "../../../content/legal", `${h}.md`))).toBe(true);
    }
  });
});

describe("legal re-acceptance mount", () => {
  const shell = readFileSync(join(__dirname, "ConsumerShell.tsx"), "utf8");

  it("[A legacy user is prompted and accepts] the shell imports and mounts LegalReacceptance", () => {
    expect(shell).toMatch(/features\/legal\/LegalReacceptance/);
    expect(shell).toMatch(/<LegalReacceptance\b/);
  });
});
