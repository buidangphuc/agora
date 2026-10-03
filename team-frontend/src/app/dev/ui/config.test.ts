import { afterEach, describe, expect, it, vi } from "vitest";

import nextConfig from "../../../../next.config.mjs";

afterEach(() => vi.unstubAllEnvs());

async function rewrites() {
  if (!nextConfig.rewrites) throw new Error("next.config has no rewrites()");
  return nextConfig.rewrites();
}

describe("next.config /dev rewrite", () => {
  it("routes /dev/* to a route that does not exist in production (real HTTP 404)", async () => {
    vi.stubEnv("NODE_ENV", "production");
    expect(await rewrites()).toEqual({
      beforeFiles: [{ source: "/dev/:path*", destination: "/__dev-disabled" }],
    });
  });

  it("leaves /dev/ui reachable in development", async () => {
    vi.stubEnv("NODE_ENV", "development");
    expect(await rewrites()).toEqual([]);
  });
});
