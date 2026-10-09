import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// ConsumerShell is an async server component with an async child (AuthNav),
// which jsdom cannot render; assert the footer's source structure instead.
describe("ConsumerShell footer", () => {
  const src = readFileSync(join(__dirname, "ConsumerShell.tsx"), "utf8");
  const footer = src.slice(
    src.indexOf("<footer"),
    src.indexOf(">", src.indexOf("className=", src.indexOf("<footer"))),
  );

  it("reserves the buy-bar height (pb-20) below lg when the page has a buy bar", () => {
    expect(footer).toContain("[body:has([data-testid=buy-bar])_&]:pb-20");
    expect(footer).toContain("lg:[body:has([data-testid=buy-bar])_&]:pb-0");
  });
});
