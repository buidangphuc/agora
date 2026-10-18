import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  ListingStatus,
  ModerationDecision,
} from "@/generated/platform/listing/v1/listing_pb.js";

import { makeClients } from "./client.js";
import { listModerationQueue, moderateListing } from "./moderation.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/moderation.ts):
//   listModerationQueue(cursor) =>
//     { status: "ok"; items: ModerationQueueItemView[]; nextCursor: string }
//   | { status: "unauthenticated" | "forbidden" | "unavailable" }
//   ModerationQueueItemView = { id, title, sellerId: string; price: number;
//     submittedAt: string /* ISO-8601 UTC */; screenFlags: string[] }
//   moderateListing(listingId, decision: "approve" | "reject" | "ban",
//                   reasonCode, note) =>
//     { status: "ok" } | { status: "unauthenticated" | "forbidden" | "not_found"
//       | "failed_precondition" | "invalid" | "unavailable" }
//   Results never carry err.message.

function stub(listing: Record<string, ReturnType<typeof vi.fn>>) {
  vi.mocked(makeClients).mockReturnValue({ listing } as never);
  return listing;
}

function item(id: string, iso: string, flags: string[] = []) {
  return {
    listing: {
      id,
      title: `T-${id}`,
      sellerId: "s-1",
      price: 120000n,
      currency: "VND",
      status: ListingStatus.PENDING_REVIEW,
      moderation: { reasonCode: "", note: "", screenFlags: flags },
    },
    submittedAt: Timestamp.fromDate(new Date(iso)),
  };
}

beforeEach(() => vi.clearAllMocks());

describe("listModerationQueue gateway wrapper", () => {
  // Scenario: Moderation UI: Server order is kept and page size is 20
  it("sends page size 20 and keeps order", async () => {
    const listing = stub({
      listModerationQueue: vi.fn().mockResolvedValue({
        items: [
          item("a", "2026-10-01T00:00:00Z", ["keyword:x"]),
          item("b", "2026-10-02T00:00:00Z"),
          item("c", "2026-10-03T00:00:00Z"),
        ],
        page: { nextCursor: "20" },
      }),
    });
    const res = await listModerationQueue("");
    const [req, opts] = listing.listModerationQueue.mock.calls[0];
    expect(req.page).toMatchObject({ cursor: "", pageSize: 20 });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res.status).toBe("ok");
    if (res.status !== "ok") return;
    expect(res.items.map((i) => i.id)).toEqual(["a", "b", "c"]);
    expect(res.items[0]).toMatchObject({
      title: "T-a",
      sellerId: "s-1",
      submittedAt: "2026-10-01T00:00:00.000Z",
      screenFlags: ["keyword:x"],
    });
    expect(res.nextCursor).toBe("20");
  });

  // Scenario: Moderation UI: Server order is kept and page size is 20
  it("forwards the cursor and returns an empty next cursor on the last page", async () => {
    const listing = stub({
      listModerationQueue: vi.fn().mockResolvedValue({ items: [], page: {} }),
    });
    const res = await listModerationQueue("40");
    expect(listing.listModerationQueue.mock.calls[0][0].page).toMatchObject({
      cursor: "40",
      pageSize: 20,
    });
    expect(res).toMatchObject({ status: "ok", items: [], nextCursor: "" });
  });

  // Scenario: Moderation UI: A forged action by a non-admin is refused
  it.each([
    [Code.Unauthenticated, "unauthenticated"],
    [Code.PermissionDenied, "forbidden"],
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "unavailable"],
  ])("queue maps code %s to %s", async (code, status) => {
    stub({
      listModerationQueue: vi
        .fn()
        .mockRejectedValue(new ConnectError("upstream 10.0.0.7:5432", code)),
    });
    const res = await listModerationQueue("");
    expect(res).toEqual({ status });
  });
});

describe("moderateListing gateway wrapper", () => {
  const rpc = () => vi.fn().mockResolvedValue({ listing: { id: "l1" } });

  // Scenario: Moderation UI: Approve publishes and removes the row
  it("approve sends one APPROVE call with a 5 s timeout", async () => {
    const listing = stub({ moderateListing: rpc() });
    const res = await moderateListing("l1", "approve", "", "");
    expect(res).toEqual({ status: "ok" });
    expect(listing.moderateListing).toHaveBeenCalledTimes(1);
    const [req, opts] = listing.moderateListing.mock.calls[0];
    expect(req).toMatchObject({
      listingId: "l1",
      decision: ModerationDecision.APPROVE,
    });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
  });

  // Scenario: Moderation UI: Reject sends reason and note
  it("reject sends decision, reason code and note", async () => {
    const listing = stub({ moderateListing: rpc() });
    await moderateListing(
      "l1",
      "reject",
      "counterfeit",
      "Ảnh sản phẩm là hàng nhái",
    );
    expect(listing.moderateListing.mock.calls[0][0]).toMatchObject({
      listingId: "l1",
      decision: ModerationDecision.REJECT,
      reasonCode: "counterfeit",
      note: "Ảnh sản phẩm là hàng nhái",
    });
  });

  // Scenario: Moderation UI: Ban removes the listing from the queue
  it("ban sends decision BAN", async () => {
    const listing = stub({ moderateListing: rpc() });
    await moderateListing("l1", "ban", "prohibited_item", "");
    expect(listing.moderateListing.mock.calls[0][0]).toMatchObject({
      decision: ModerationDecision.BAN,
      reasonCode: "prohibited_item",
    });
  });

  // Scenarios: Audit unavailable leaves the row; Not-allowed transition is
  // explained; Listing is gone; A forged action by a non-admin is refused
  it.each([
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "unavailable"],
    [Code.FailedPrecondition, "failed_precondition"],
    [Code.NotFound, "not_found"],
    [Code.PermissionDenied, "forbidden"],
    [Code.Unauthenticated, "unauthenticated"],
    [Code.InvalidArgument, "invalid"],
  ])("maps code %s to %s and never leaks err.message", async (code, status) => {
    stub({
      moderateListing: vi
        .fn()
        .mockRejectedValue(new ConnectError("secret db host pg-1:5432", code)),
    });
    const res = await moderateListing("l1", "reject", "other", "");
    expect(res).toEqual({ status });
    expect(JSON.stringify(res)).not.toContain("pg-1");
  });

  // Scenario: Moderation UI: Audit unavailable leaves the row
  it("maps a non-Connect failure to unavailable", async () => {
    stub({ moderateListing: vi.fn().mockRejectedValue(new Error("boom")) });
    expect(await moderateListing("l1", "approve", "", "")).toEqual({
      status: "unavailable",
    });
  });
});
