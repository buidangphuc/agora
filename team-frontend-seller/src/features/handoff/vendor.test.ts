import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("vendored session package", () => {
  it("[Buyer to seller signed in / Seller to buyer signed in] ships the hand-off module after npm run vendor", () => {
    const tgz = join(process.cwd(), "vendor", "fe-platform-session.tgz");
    const listing = execFileSync("tar", ["-tzf", tgz], { encoding: "utf8" });
    expect(listing).toMatch(/handoff/);
  });
});
