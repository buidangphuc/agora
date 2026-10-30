import { describe, expect, it } from "vitest";
import { jwt, nowSec } from "./__helpers__/identity";
import { decodePrincipal, hasScopes, isUsableToken, tokenState } from "./token";

describe("tokenState", () => {
  it("classifies usable, expired and malformed tokens", () => {
    expect(tokenState(jwt({ exp: nowSec() + 3600 }))).toBe("usable");
    expect(tokenState(jwt({ exp: nowSec() - 60 }))).toBe("expired");
    expect(tokenState(jwt({ exp: nowSec() + 2 }))).toBe("expired");
    expect(tokenState("not-a-jwt")).toBe("malformed");
    expect(tokenState("a.b.c")).toBe("malformed");
    expect(isUsableToken(jwt({}))).toBe(true);
    expect(isUsableToken("x.y")).toBe(false);
  });
});

describe("decodePrincipal and hasScopes", () => {
  it("reads id, name and scopes", () => {
    const p = decodePrincipal(
      jwt({ sub: "u9", name: "An", scopes: ["listing.write", "x"] }),
    );
    expect(p).toMatchObject({
      id: "u9",
      name: "An",
      scopes: ["listing.write", "x"],
    });
    expect(decodePrincipal("bad")).toBeNull();
  });

  it("ignores non-string scopes and checks every required scope", () => {
    const p = decodePrincipal(jwt({ scopes: ["a", 3 as unknown as string] }));
    expect(p?.scopes).toEqual(["a"]);
    expect(hasScopes(p, ["a"])).toBe(true);
    expect(hasScopes(p, ["a", "b"])).toBe(false);
    expect(hasScopes(null, [])).toBe(false);
    expect(hasScopes({ scopes: [] }, [])).toBe(true);
  });
});
