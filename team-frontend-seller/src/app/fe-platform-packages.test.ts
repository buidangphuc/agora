import { ApiError, toApiError } from "@fe-platform/api-client";
import { SESSION_PRESETS, cookieNames } from "@fe-platform/session";
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

  it("resolve the session package from the vendored tarball", () => {
    expect(cookieNames(SESSION_PRESETS.buyer, true).access).toBe("session");
    expect(cookieNames(SESSION_PRESETS.admin, true).access).toBe(
      "__Host-admin_session",
    );
  });
});
