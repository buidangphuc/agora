// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const dockerfile = readFileSync(
  new URL("../../Dockerfile", import.meta.url),
  "utf8",
);

describe("Dockerfile and vendored packages", () => {
  it("copies vendor/ before npm ci so file: dependencies resolve", () => {
    const copy = dockerfile.indexOf("COPY vendor ./vendor");
    const install = dockerfile.indexOf("npm ci");
    expect(copy).toBeGreaterThan(-1);
    expect(install).toBeGreaterThan(-1);
    expect(copy).toBeLessThan(install);
  });
});
