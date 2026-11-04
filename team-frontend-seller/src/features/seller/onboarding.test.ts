import { Code, ConnectError } from "@connectrpc/connect";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { acceptSellerTerms } from "./onboarding";

const seen = vi.hoisted(() => ({
  calls: [] as { headers: Headers; message: unknown; method: string }[],
  reply: (() => ({})) as () => unknown,
}));

vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: (opts: {
    interceptors?: ((
      next: (r: unknown) => Promise<unknown>,
    ) => (r: unknown) => Promise<unknown>)[];
  }) => ({
    async unary(
      service: unknown,
      method: { name: string },
      _signal: unknown,
      _timeoutMs: number | undefined,
      header: HeadersInit | undefined,
      message: unknown,
    ) {
      const terminal = async (req: { header: Headers }) => {
        seen.calls.push({
          headers: req.header,
          message,
          method: method.name,
        });
        return {
          message: seen.reply(),
          header: new Headers(),
          trailer: new Headers(),
        };
      };
      let fn = terminal as (r: never) => Promise<unknown>;
      for (const i of [...(opts.interceptors ?? [])].reverse()) {
        fn = i(fn as never) as never;
      }
      return (await fn({
        header: new Headers(header),
        service,
        method,
        message,
      } as never)) as { message: unknown };
    },
    stream: vi.fn(),
  }),
}));

function jwt(tag: string): string {
  const b = (o: object) =>
    btoa(JSON.stringify(o))
      .replace(/=+$/, "")
      .replace(/\+/g, "-")
      .replace(/\//g, "_");
  return `${b({ alg: "RS256" })}.${b({ sub: "u1", exp: 4102444800, tag })}.sig${tag}`;
}

const OLD = jwt("old");
const NEW = jwt("new");
const VERSION = "2026-11-02";

async function setCookies() {
  const jar = await cookies();
  return vi.mocked(jar.set).mock.calls as unknown as [
    string,
    string,
    Record<string, unknown>?,
  ][];
}

async function withSession(token: string | null) {
  const jar = await cookies();
  jar.delete("seller_session");
  jar.delete("seller_refresh");
  if (token) jar.set("seller_session", token);
  vi.mocked(jar.set).mockClear();
  vi.mocked(jar.delete).mockClear();
}

beforeEach(async () => {
  vi.clearAllMocks();
  seen.calls = [];
  seen.reply = () => ({ result: { token: NEW } });
  await withSession(OLD);
});

function fail(code: Code, msg = "boom") {
  seen.reply = () => {
    throw new ConnectError(msg, code);
  };
}

describe("acceptSellerTerms", () => {
  it("[Seller onboarding UI: accepting sends the displayed version and updates the session cookie] accept sets only the session cookie and redirects", async () => {
    await acceptSellerTerms(VERSION);

    const rpcs = seen.calls.filter((c) => c.method === "BecomeSeller");
    expect(rpcs).toHaveLength(1);
    expect(seen.calls).toHaveLength(1);
    expect(rpcs[0].message).toMatchObject({ termsVersion: VERSION });
    expect(rpcs[0].headers.get("authorization")).toBe(`Bearer ${OLD}`);

    const sets = await setCookies();
    expect(sets.map((c) => c[0])).toEqual(["seller_session"]);
    expect(sets[0][1]).toBe(NEW);
    expect(sets[0][2]).toEqual(
      expect.objectContaining({ httpOnly: true, path: "/" }),
    );
    expect(redirect).toHaveBeenCalledTimes(1);
    expect(redirect).toHaveBeenCalledWith("/seller/new");
  });

  it("[Seller onboarding UI: accepting sends the displayed version and updates the session cookie] a different displayed version is the one sent", async () => {
    await acceptSellerTerms("2027-01-01");
    expect(seen.calls[0].message).toMatchObject({ termsVersion: "2027-01-01" });
  });

  it("[Seller onboarding UI: the access token never reaches the client] token is not exposed to the client", async () => {
    const result = await acceptSellerTerms(VERSION);
    const body = JSON.stringify(result ?? null);
    expect(body).not.toContain(NEW);
    expect(body).not.toContain("signew");
    expect(body).not.toContain(OLD);
    if (result && typeof result === "object") {
      for (const key of Object.keys(result)) {
        expect(["ok", "error"]).toContain(key);
      }
    }
  });

  it("[Seller onboarding UI: the accept action refuses to run without a session] accept without session sets no cookie", async () => {
    await withSession(null);
    const result = await acceptSellerTerms(VERSION);

    expect(seen.calls).toHaveLength(0);
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
    expect(result).toMatchObject({ error: expect.any(String) });
    expect((result as { error: string }).error).not.toBe("");
    expect((result as { error: string }).error).not.toBe("denied");
  });

  it("[Seller onboarding UI: an unknown terms version shows an error] invalid terms version shows error and keeps cookies", async () => {
    fail(Code.InvalidArgument, "unknown terms version");
    const result = (await acceptSellerTerms("bogus")) as { error: string };

    expect(seen.calls).toHaveLength(1);
    expect(result.error).toBeTruthy();
    expect(result.error).not.toBe("denied");
    expect(JSON.stringify(result)).not.toContain("unknown terms version");
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
  });

  it("[Seller onboarding UI: an unavailable gateway shows an error] unavailable shows error and keeps cookies", async () => {
    fail(Code.Unavailable, "upstream down");
    const result = (await acceptSellerTerms(VERSION)) as { error: string };

    expect(result.error).toBeTruthy();
    expect(result.error).not.toBe("denied");
    expect(JSON.stringify(result)).not.toContain("upstream down");
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
  });

  it("[Seller onboarding UI: an unavailable gateway shows an error] a non-connect failure shows an error and keeps cookies", async () => {
    seen.reply = () => {
      throw new Error("dns");
    };
    const result = (await acceptSellerTerms(VERSION)) as { error: string };

    expect(result.error).toBeTruthy();
    expect(result.error).not.toBe("denied");
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
  });

  it("[Seller onboarding UI: an expired session on accept goes to login] unauthenticated redirects to login", async () => {
    fail(Code.Unauthenticated, "session revoked");
    await acceptSellerTerms(VERSION);

    expect(redirect).toHaveBeenCalledTimes(1);
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/seller/new")}`,
    );
    expect(await setCookies()).toHaveLength(0);
  });

  it("[Seller onboarding UI: a denied acceptance shows the denied notice] permission denied shows fixed denied notice", async () => {
    fail(Code.PermissionDenied, "account restricted");
    const result = await acceptSellerTerms(VERSION);

    expect(result).toEqual({ error: "denied" });
    expect(JSON.stringify(result)).not.toContain("account restricted");
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
  });

  it("[Seller onboarding UI: an empty token is an error] empty token shows error and keeps cookies", async () => {
    seen.reply = () => ({ result: { token: "" } });
    const result = (await acceptSellerTerms(VERSION)) as { error: string };

    expect(result.error).toBeTruthy();
    expect(result.error).not.toBe("denied");
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
    const jar = await cookies();
    expect(jar.get("seller_session")?.value).toBe(OLD);
  });

  it("[Seller onboarding UI: an empty token is an error] a response without a result is an error", async () => {
    seen.reply = () => ({});
    const result = (await acceptSellerTerms(VERSION)) as { error: string };

    expect(result.error).toBeTruthy();
    expect(await setCookies()).toHaveLength(0);
    expect(redirect).not.toHaveBeenCalled();
  });
});
