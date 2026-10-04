import { Writable } from "node:stream";
import { renderToPipeableStream } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

// Async server components cannot render in the plain React DOM server, so the
// slow block is modelled the way Suspense sees it: it suspends until released.
const gate = vi.hoisted(() => {
  let release: () => void = () => {};
  let released = false;
  const promise = new Promise<void>((r) => {
    release = () => {
      released = true;
      r();
    };
  });
  return { promise, release: () => release(), isReleased: () => released };
});

vi.mock("@/features/home/FeedBlock", () => ({
  FeedBlock: () => {
    if (!gate.isReleased()) throw gate.promise;
    return <p>FEED_CONTENT</p>;
  },
}));
vi.mock("@/features/home/CategoryGridBlock", () => ({
  CategoryGridBlock: () => <p>CATEGORY_CONTENT</p>,
}));
vi.mock("@/features/home/FlashSaleBlock", () => ({
  FlashSaleBlock: () => null,
}));
vi.mock("@/features/home/LoyaltyBlock", () => ({
  LoyaltyBlock: () => null,
}));
vi.mock("@/features/home/AssistantBlock", () => ({
  AssistantBlock: () => null,
}));
vi.mock("@/features/home/RecentlyViewedRow", () => ({
  RecentlyViewedRow: () => null,
}));
vi.mock("@/features/recommendations/RecommendationsRow", () => ({
  RecommendationsRow: () => null,
}));

import HomePage from "./page";

function collect(): { sink: Writable; text: () => string } {
  let out = "";
  const sink = new Writable({
    write(chunk, _enc, cb) {
      out += chunk.toString();
      cb();
    },
  });
  return { sink, text: () => out };
}

describe("HomePage streaming", () => {
  it("shows hero, hubs and categories while a slow feed shows its skeleton", async () => {
    const { sink, text } = collect();
    let shell = "";
    const done = new Promise<void>((resolve) => {
      const stream = renderToPipeableStream(<HomePage />, {
        onShellReady() {
          stream.pipe(sink);
        },
        onAllReady() {
          resolve();
        },
      });
      setTimeout(() => {
        shell = text();
        gate.release();
      }, 50);
    });

    await done;
    // Shell flushed before the feed resolved.
    expect(shell).toContain("Mua ngay");
    expect(shell).toContain("Kho voucher");
    expect(shell).toContain("CATEGORY_CONTENT");
    expect(shell).not.toContain("FEED_CONTENT");
    expect(shell).toContain('aria-busy="true"');
    // Everything arrives once the feed resolves.
    expect(text()).toContain("FEED_CONTENT");
  });
});
