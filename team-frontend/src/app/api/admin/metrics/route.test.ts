import { beforeEach, describe, expect, it, vi } from "vitest";

import { fetchCockpit } from "@/lib/gateway/cockpit";
import { getToken } from "@/lib/gateway/session";
import { GET } from "./route";

vi.mock("@/lib/gateway/session", () => ({ getToken: vi.fn() }));
vi.mock("@/lib/gateway/cockpit", () => ({ fetchCockpit: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("GET /api/admin/metrics (client refresh)", () => {
  it("is 401 without a session and never calls the gateway", async () => {
    vi.mocked(getToken).mockReturnValue(undefined);
    const res = await GET();
    expect(res.status).toBe(401);
    expect(fetchCockpit).not.toHaveBeenCalled();
  });

  it("forwards the session token and returns the payload", async () => {
    vi.mocked(getToken).mockReturnValue("tok");
    vi.mocked(fetchCockpit).mockResolvedValue({
      status: "ok",
      data: { total_rps: 1 } as never,
    });
    const res = await GET();
    expect(fetchCockpit).toHaveBeenCalledWith("tok");
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ total_rps: 1 });
  });

  it("passes the gateway's 403 through", async () => {
    vi.mocked(getToken).mockReturnValue("tok");
    vi.mocked(fetchCockpit).mockResolvedValue({ status: "forbidden" });
    expect((await GET()).status).toBe(403);
  });

  it("is 502 when the gateway is unavailable", async () => {
    vi.mocked(getToken).mockReturnValue("tok");
    vi.mocked(fetchCockpit).mockResolvedValue({ status: "unavailable" });
    expect((await GET()).status).toBe(502);
  });
});
