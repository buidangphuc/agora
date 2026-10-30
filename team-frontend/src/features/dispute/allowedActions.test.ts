import type { ViewDispute } from "@/lib/gateway/disputes";
import { describe, expect, it } from "vitest";
import { isOpenDispute } from "./allowedActions";

function dispute(over: Partial<ViewDispute> = {}): ViewDispute {
  return {
    id: "d1",
    orderId: "o1",
    claimantId: "u1",
    defendantId: "u2",
    reason: "r",
    resolution: "",
    returnId: "",
    evidenceUrls: [],
    status: "OPEN",
    statusText: "",
    ruling: "",
    rulingText: "",
    createdAt: "",
    ...over,
  };
}

describe("isOpenDispute (Scenario: Actions offered per status)", () => {
  it("is true for OPEN and INVESTIGATING, false once closed", () => {
    expect(isOpenDispute(dispute({ status: "OPEN" }))).toBe(true);
    expect(isOpenDispute(dispute({ status: "INVESTIGATING" }))).toBe(true);
    expect(isOpenDispute(dispute({ status: "RESOLVED" }))).toBe(false);
    expect(isOpenDispute(dispute({ status: "REJECTED" }))).toBe(false);
  });
});
