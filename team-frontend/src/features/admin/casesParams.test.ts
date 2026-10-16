import { describe, expect, it } from "vitest";

import { parseCasesParams } from "./casesParams";

// Contract under test (implementer creates src/features/admin/casesParams.ts):
//   parseCasesParams(searchParams) -> { tab: "reports" | "queue",
//     status: "OPEN" | "ACTIONED" | "DISMISSED", cursor: string }
// Never throws; unknown values fall back; arrays take the first element;
// strings are trimmed; cursor is capped at 256 characters.

describe("parseCasesParams", () => {
  it("defaults to the reports tab with status OPEN (Moderation UI: Default tab is reports with status OPEN)", () => {
    expect(parseCasesParams({})).toEqual({
      tab: "reports",
      status: "OPEN",
      cursor: "",
    });
  });

  it("falls back on unknown tab and status (Moderation UI: Unknown tab or status falls back)", () => {
    expect(parseCasesParams({ tab: "evil", status: "bogus" })).toEqual({
      tab: "reports",
      status: "OPEN",
      cursor: "",
    });
  });

  it("accepts the queue tab and each report status (Moderation UI: Unknown tab or status falls back)", () => {
    expect(parseCasesParams({ tab: "queue" }).tab).toBe("queue");
    expect(parseCasesParams({ status: "ACTIONED" }).status).toBe("ACTIONED");
    expect(parseCasesParams({ status: "DISMISSED" }).status).toBe("DISMISSED");
    expect(parseCasesParams({ status: "OPEN" }).status).toBe("OPEN");
  });

  it("takes the first element of an array value and trims (Moderation UI: Unknown tab or status falls back)", () => {
    const p = parseCasesParams({
      tab: ["queue", "reports"],
      status: ["DISMISSED", "OPEN"],
      cursor: ["  20  ", "40"],
    });
    expect(p).toEqual({ tab: "queue", status: "DISMISSED", cursor: "20" });
  });

  it("caps the cursor at 256 characters (Moderation UI: Unknown tab or status falls back)", () => {
    expect(parseCasesParams({ cursor: "x".repeat(300) }).cursor).toHaveLength(
      256,
    );
    expect(parseCasesParams({ cursor: "   " }).cursor).toBe("");
  });
});
