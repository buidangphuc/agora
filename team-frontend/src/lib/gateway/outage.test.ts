import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
  listSessions: vi.fn(),
  listLoginHistory: vi.fn(),
  listNotifications: vi.fn(),
}));

vi.mock("server-only", () => ({}));
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("@connectrpc/connect", async (orig) => ({
  ...(await orig<typeof import("@connectrpc/connect")>()),
  createPromiseClient: vi.fn(() => rpc),
}));
vi.mock("./session.js", () => ({ getToken: vi.fn(() => "t") }));
vi.mock("./config.js", () => ({
  gatewayConfig: { gatewayUrl: "http://gw" },
}));

import { listNotifications } from "./notification.js";
import { listLoginHistory, listSessions } from "./sessions.js";

beforeEach(() => vi.clearAllMocks());

describe("reads distinguish an outage from an empty result", () => {
  it("listSessions: empty list stays empty, an RPC error throws", async () => {
    rpc.listSessions.mockResolvedValueOnce({ sessions: [] });
    await expect(listSessions()).resolves.toEqual([]);
    rpc.listSessions.mockRejectedValueOnce(new Error("down"));
    await expect(listSessions()).rejects.toThrow("down");
  });

  it("listLoginHistory: empty list stays empty, an RPC error throws", async () => {
    rpc.listLoginHistory.mockResolvedValueOnce({ events: [] });
    await expect(listLoginHistory()).resolves.toEqual([]);
    rpc.listLoginHistory.mockRejectedValueOnce(new Error("down"));
    await expect(listLoginHistory()).rejects.toThrow("down");
  });

  it("listNotifications: empty stays empty, an RPC error throws", async () => {
    rpc.listNotifications.mockResolvedValueOnce({
      notifications: [],
      totalUnread: 0,
    });
    await expect(listNotifications()).resolves.toEqual({
      notifications: [],
      totalUnread: 0,
    });
    rpc.listNotifications.mockRejectedValueOnce(new Error("down"));
    await expect(listNotifications()).rejects.toThrow("down");
  });
});
