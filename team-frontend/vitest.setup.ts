import "@testing-library/jest-dom/vitest";

import { vi } from "vitest";

/**
 * Global stubs for the Next.js server APIs the Server Actions and gateway
 * session helper reach for. Declared here so every test file gets them without
 * repeating the boilerplate; individual tests can still inspect or override the
 * returned vi.fn()s (e.g. read what `cookies().set` was called with).
 *
 * `next/headers` cookies() is backed by a per-test-file in-memory jar so
 * getToken()/setSession() round-trip like the real cookie store.
 */
vi.mock("next/headers", () => {
  const store = new Map<string, string>();
  const jar = {
    get: vi.fn((name: string) => {
      const value = store.get(name);
      return value === undefined ? undefined : { name, value };
    }),
    getAll: vi.fn(() =>
      [...store.entries()].map(([name, value]) => ({ name, value })),
    ),
    has: vi.fn((name: string) => store.has(name)),
    set: vi.fn((name: string, value: string) => {
      store.set(name, value);
    }),
    delete: vi.fn((name: string) => {
      store.delete(name);
    }),
  };
  return {
    cookies: vi.fn(() => jar),
    headers: vi.fn(() => new Map<string, string>()),
  };
});

vi.mock("next/cache", () => ({
  revalidatePath: vi.fn(),
  revalidateTag: vi.fn(),
}));

// `redirect` in Next throws a control-flow signal to halt rendering; the stub is
// a plain spy so actions that call it simply resolve, and tests can assert the
// target path.
vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
