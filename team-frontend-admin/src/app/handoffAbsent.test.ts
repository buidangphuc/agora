import { existsSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("admin app", () => {
  it("[Admin never hands off] admin app has no hand-off route", () => {
    expect(existsSync(join(__dirname, "auth", "handoff"))).toBe(false);
    expect(existsSync(join(__dirname, "api", "auth", "handoff"))).toBe(false);
  });
});
