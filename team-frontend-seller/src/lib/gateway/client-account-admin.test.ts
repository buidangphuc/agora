import { describe, expect, it } from "vitest";

import {
  ListUsersRequest,
  SetUserStatusRequest,
  UserStatus,
} from "@/generated/platform/identity/v1/identity_pb.js";

import { makeClients } from "./client.js";

// Task 1.1. Scenario: "Users UI: Search is forwarded as typed fields".
// The gateway client set must expose an AccountAdminService client as
// `accountAdmin`, and the generated request types the later tasks build on
// must carry the typed query / status / page fields.

describe("makeClients accountAdmin (Users UI: Search is forwarded as typed fields)", () => {
  it("exposes an accountAdmin client with listUsers and setUserStatus", () => {
    const clients = makeClients("token") as unknown as Record<
      string,
      Record<string, unknown>
    >;
    expect(clients.accountAdmin).toBeDefined();
    expect(typeof clients.accountAdmin.listUsers).toBe("function");
    expect(typeof clients.accountAdmin.setUserStatus).toBe("function");
  });

  it("exposes accountAdmin for an anonymous client too (Users UI: Search is forwarded as typed fields)", () => {
    const clients = makeClients() as unknown as Record<string, unknown>;
    expect(clients.accountAdmin).toBeDefined();
  });

  it("generated request types carry typed fields (Users UI: Search is forwarded as typed fields)", () => {
    const list = new ListUsersRequest({
      query: "ann",
      status: UserStatus.SUSPENDED,
    });
    expect(list.query).toBe("ann");
    expect(list.status).toBe(UserStatus.SUSPENDED);
    const set = new SetUserStatusRequest({
      userId: "u1",
      status: UserStatus.BANNED,
    });
    expect(set.userId).toBe("u1");
    expect(set.status).toBe(UserStatus.BANNED);
  });
});
