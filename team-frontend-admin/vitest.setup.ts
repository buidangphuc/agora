import "@testing-library/jest-dom/vitest";

import { vi } from "vitest";

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

if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

vi.mock("next/cache", () => ({
  revalidatePath: vi.fn(),
  revalidateTag: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
