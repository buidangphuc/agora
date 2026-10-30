import { ApiError, toApiError } from "@fe-platform/api-client";
import { components, primitives } from "@fe-platform/tokens";
import { Button, Card } from "@fe-platform/ui";
import { describe, expect, it } from "vitest";

describe("vendored fe-platform packages", () => {
  it("resolve tokens from the vendored tarball", () => {
    expect(components.button.base).toContain("min-h-touch");
    expect(primitives.touchTarget).toBe(44);
  });

  it("resolve ui components from the vendored tarball", () => {
    expect(typeof Button).toBe("function");
    expect(typeof Card).toBe("function");
  });

  it("resolve the api-client from the vendored tarball", () => {
    expect(toApiError(new Error("x"))).toBeInstanceOf(ApiError);
  });
});
