import { render } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce } = vi.hoisted(() => ({ trackEcommerce: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));

const nav = vi.hoisted(() => ({ pathname: "/login" }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  usePathname: () => nav.pathname,
}));

import { AuthEventBeacon } from "./AuthEventBeacon";

const NAME = "agora_auth_evt";
const clear = () => {
  document.cookie = `${NAME}=; Max-Age=0; Path=/`;
};

beforeEach(() => {
  trackEcommerce.mockReset();
  nav.pathname = "/login";
  clear();
});
afterEach(() => {
  clear();
  // remove any instance-level cookie stub
  Reflect.deleteProperty(document, "cookie");
});

describe("AuthEventBeacon", () => {
  it("fires once and deletes the cookie", () => {
    document.cookie = `${NAME}=login; Path=/`;
    let cookieAtCall = "unset";
    trackEcommerce.mockImplementation(() => {
      cookieAtCall = document.cookie;
    });

    render(
      <StrictMode>
        <AuthEventBeacon />
      </StrictMode>,
    );

    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("login", {
      properties: { method: "password" },
    });
    // cookie was already expired when trackEcommerce ran
    expect(cookieAtCall).not.toContain(NAME);
    expect(document.cookie).not.toContain(NAME);
  });

  it("fires once and deletes the cookie: sign_up value", () => {
    document.cookie = `${NAME}=sign_up; Path=/`;
    render(<AuthEventBeacon />);
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("sign_up", {
      properties: { method: "password" },
    });
  });

  it("ignores unknown values: no cookie", () => {
    render(<AuthEventBeacon />);
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("ignores unknown values: admin and empty are expired without tracking", () => {
    for (const value of ["admin", ""]) {
      document.cookie = `${NAME}=${value}; Path=/`;
      const { unmount } = render(<AuthEventBeacon />);
      unmount();
      expect(document.cookie).not.toContain(NAME);
    }
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("ignores unknown values: an oversized value injected via a document.cookie stub", () => {
    const writes: string[] = [];
    Object.defineProperty(document, "cookie", {
      configurable: true,
      get: () => `${NAME}=${"a".repeat(4001)}`,
      set: (v: string) => {
        writes.push(v);
      },
    });
    render(<AuthEventBeacon />);
    expect(trackEcommerce).not.toHaveBeenCalled();
    expect(
      writes.some((w) => w.startsWith(`${NAME}=`) && /Max-Age=0/i.test(w)),
    ).toBe(true);
  });

  it("fires after a soft navigation", () => {
    const { rerender } = render(<AuthEventBeacon />);
    expect(trackEcommerce).not.toHaveBeenCalled();

    document.cookie = `${NAME}=login; Path=/`;
    nav.pathname = "/";
    rerender(<AuthEventBeacon />);

    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("login", {
      properties: { method: "password" },
    });
    expect(document.cookie).not.toContain(NAME);
  });
});
