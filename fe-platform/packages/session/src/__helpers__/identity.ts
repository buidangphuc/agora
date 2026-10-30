export function jwt(
  claims: { exp?: number; sub?: string; name?: string; scopes?: string[] } = {},
): string {
  const enc = (o: object) =>
    btoa(JSON.stringify(o))
      .replace(/=+$/, "")
      .replace(/\+/g, "-")
      .replace(/\//g, "_");
  return `${enc({ alg: "RS256" })}.${enc({ sub: "u1", ...claims })}.sig`;
}

export const nowSec = () => Math.floor(Date.now() / 1000);

export class RejectedError extends Error {
  readonly code = 16;
}

interface Session {
  current: string;
  used: Set<string>;
  revoked: boolean;
}

export class FakeIdentity {
  readonly sessions = new Map<string, Session>();
  private n = 0;

  login(): string {
    const refresh = `R${++this.n}`;
    this.sessions.set(refresh, {
      current: refresh,
      used: new Set(),
      revoked: false,
    });
    return refresh;
  }

  async refresh(refreshToken: string) {
    const owner = [...this.sessions.values()].find(
      (s) => s.current === refreshToken || s.used.has(refreshToken),
    );
    if (!owner || owner.revoked) throw new RejectedError("unknown token");
    if (owner.used.has(refreshToken)) {
      owner.revoked = true;
      throw new RejectedError("reused token");
    }
    owner.used.add(refreshToken);
    owner.current = `R${++this.n}`;
    return {
      token: jwt({ exp: nowSec() + 3600, sub: `session-${this.n}` }),
      refreshToken: owner.current,
    };
  }

  revokedCount(): number {
    return [...this.sessions.values()].filter((s) => s.revoked).length;
  }
}
