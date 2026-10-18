import { readFileSync } from "node:fs";
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

// Moderation UI: Cockpit shows both links (footer half of the admin entry links)
describe("admin footer links", () => {
  const shell = readFileSync(join(__dirname, "ConsumerShell.tsx"), "utf8");
  const footer = shell.slice(shell.indexOf("<footer"));

  function linkFor(href: string) {
    const re = new RegExp(
      `<Link\\s+href="${href}"[^>]*>\\s*([^<]*?)\\s*</Link>`,
    );
    return footer.match(re)?.[1];
  }

  it("links Kiểm duyệt tin đăng to /admin/listings (Moderation UI: Cockpit shows both links)", () => {
    expect(linkFor("/admin/listings")).toBe("Kiểm duyệt tin đăng");
  });

  it("links Hộp thư vụ việc to /admin/cases (Moderation UI: Cockpit shows both links)", () => {
    expect(linkFor("/admin/cases")).toBe("Hộp thư vụ việc");
  });

  it("keeps the cockpit link (Moderation UI: Cockpit shows both links)", () => {
    expect(linkFor("/admin/cockpit")).toBeTruthy();
  });
});
