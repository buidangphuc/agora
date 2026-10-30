import { beforeEach, describe, expect, it, vi } from "vitest";

const redirectToLogin = vi.hoisted(() => vi.fn());

vi.mock("./returnUrl", () => ({ redirectToLogin }));

import { buildReturnPath } from "./guardPath";

beforeEach(() => {
  redirectToLogin.mockReset();
  redirectToLogin.mockImplementation(() => {
    throw new Error("NEXT_REDIRECT");
  });
});

describe("Referrer hygiene: non-admin guard still preserves the query", () => {
  it("non-admin query preserved", () => {
    expect(buildReturnPath("/chat", { thread: "t1", tag: ["a", "b"] })).toBe(
      "/chat?thread=t1&tag=a&tag=b",
    );
  });
});
