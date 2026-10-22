import type { ViewDispute } from "@/lib/gateway/disputes";
import { describe, expect, it } from "vitest";
import {
  allowedDisputeActions,
  isOpenDispute,
  needsRuling,
} from "./allowedActions";

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

describe("allowedDisputeActions (Scenario: Actions offered per status)", () => {
  it("offers the server state machine only", () => {
    expect(allowedDisputeActions(dispute({ status: "OPEN" }))).toEqual([
      "investigate",
      "reject",
    ]);
    expect(allowedDisputeActions(dispute({ status: "INVESTIGATING" }))).toEqual(
      ["note", "resolve", "reject"],
    );
    expect(allowedDisputeActions(dispute({ status: "RESOLVED" }))).toEqual([]);
    expect(allowedDisputeActions(dispute({ status: "REJECTED" }))).toEqual([]);
  });

  it("never offers resolve on an OPEN dispute (Scenario: Actions offered per status)", () => {
    expect(allowedDisputeActions(dispute({ status: "OPEN" }))).not.toContain(
      "resolve",
    );
  });

  it("does not depend on return_id (Scenario: Actions offered per status)", () => {
    expect(
      allowedDisputeActions(dispute({ status: "OPEN", returnId: "r1" })),
    ).toEqual(["investigate", "reject"]);
    expect(
      allowedDisputeActions(
        dispute({ status: "INVESTIGATING", returnId: "r1" }),
      ),
    ).toEqual(["note", "resolve", "reject"]);
  });
});

describe("needsRuling (Scenario: Ruling is required for a return-linked dispute)", () => {
  it("requires a ruling for a return-linked dispute", () => {
    expect(
      needsRuling(dispute({ status: "INVESTIGATING", returnId: "r1" })),
    ).toBe(true);
  });
});

describe("needsRuling (Scenario: Dispute without a return takes no ruling)", () => {
  it("takes no ruling when return_id is empty", () => {
    expect(
      needsRuling(dispute({ status: "INVESTIGATING", returnId: "" })),
    ).toBe(false);
  });
});

describe("isOpenDispute (Scenario: Actions offered per status)", () => {
  it("is true for OPEN and INVESTIGATING, false once closed", () => {
    expect(isOpenDispute(dispute({ status: "OPEN" }))).toBe(true);
    expect(isOpenDispute(dispute({ status: "INVESTIGATING" }))).toBe(true);
    expect(isOpenDispute(dispute({ status: "RESOLVED" }))).toBe(false);
    expect(isOpenDispute(dispute({ status: "REJECTED" }))).toBe(false);
  });
});
