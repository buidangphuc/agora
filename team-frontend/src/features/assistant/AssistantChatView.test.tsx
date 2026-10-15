import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { AnchorHTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { trackAiInteraction } from "@/lib/analytics/ai";
import { AssistantChatView } from "./AssistantChatView";
import { askAssistantAction } from "./actions";

vi.mock("./actions", () => ({ askAssistantAction: vi.fn() }));
vi.mock("@/lib/analytics/ai", () => ({ trackAiInteraction: vi.fn() }));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    onClick,
    ...rest
  }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    href: string;
    children?: ReactNode;
  }) => (
    <a
      href={href}
      {...rest}
      onClick={(e) => {
        e.preventDefault();
        onClick?.(e);
      }}
    >
      {children}
    </a>
  ),
}));

function events(action?: string) {
  return vi
    .mocked(trackAiInteraction)
    .mock.calls.map(([surface, a, listingId]) => ({
      surface,
      action: a,
      listingId,
    }))
    .filter((e) => action === undefined || e.action === action);
}

function card(id: string, title: string) {
  return {
    listingId: id,
    title,
    price: 1000,
    currency: "VND",
    imageUrl: "http://img/x.png",
    discountRate: 0,
    ratingText: "4.5",
  };
}

function reply(cards: ReturnType<typeof card>[], text = "Đây là gợi ý") {
  return {
    ok: true as const,
    message: "",
    reply: { replyText: text, productCards: cards, suggestedFollowups: [] },
  };
}

async function ask(text: string) {
  fireEvent.change(screen.getByPlaceholderText(/Nhập câu hỏi/), {
    target: { value: text },
  });
  fireEvent.click(screen.getByRole("button", { name: /Gửi/ }));
}

function streamingFetch(chunks: string[]) {
  const enc = new TextEncoder();
  const queue = chunks.map((c) => enc.encode(c));
  return vi.fn().mockResolvedValue({
    ok: true,
    body: {
      getReader: () => ({
        read: async () =>
          queue.length
            ? { done: false, value: queue.shift() }
            : { done: true, value: undefined },
      }),
    },
  });
}

let now = 1_000_000;
beforeEach(() => {
  vi.clearAllMocks();
  now = 1_000_000;
  vi.spyOn(Date, "now").mockImplementation(() => {
    now += 1000;
    return now;
  });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, body: null }));
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("AssistantChatView ai_interaction", () => {
  it("emits nothing for the greeting alone", () => {
    render(<AssistantChatView />);
    expect(events()).toEqual([]);
  });

  // Scenarios: A reply with product cards reports shown once; Two replies report two shown events
  it("reports shown once per reply with cards", async () => {
    vi.mocked(askAssistantAction)
      .mockResolvedValueOnce(
        reply([card("lst_1", "Tai nghe A"), card("lst_2", "Tai nghe B")]),
      )
      .mockResolvedValueOnce(reply([card("lst_3", "Laptop C")]));
    render(<AssistantChatView />);

    await ask("tai nghe");
    await screen.findByText("Tai nghe A");
    await waitFor(() => expect(events("shown")).toHaveLength(1));
    expect(events("shown")[0]).toEqual({
      surface: "assistant",
      action: "shown",
      listingId: undefined,
    });

    // Re-render (typing in the input) does not re-report.
    fireEvent.change(screen.getByPlaceholderText(/Nhập câu hỏi/), {
      target: { value: "abc" },
    });
    fireEvent.change(screen.getByPlaceholderText(/Nhập câu hỏi/), {
      target: { value: "abcd" },
    });
    expect(events("shown")).toHaveLength(1);

    // Second reply with cards: exactly one more, first reply not re-reported.
    await ask("laptop");
    await screen.findByText("Laptop C");
    await waitFor(() => expect(events("shown")).toHaveLength(2));
    expect(events()).toHaveLength(2);
  });

  // Scenario: A reply without cards reports nothing
  it("reports nothing without cards", async () => {
    vi.mocked(askAssistantAction)
      .mockResolvedValueOnce(reply([], "Chỉ có chữ"))
      .mockResolvedValueOnce({
        ok: false,
        message: "Lỗi AI",
      } as never);
    render(<AssistantChatView />);

    await ask("xin chào");
    await screen.findByText("Chỉ có chữ");
    await ask("lỗi nhé");
    await screen.findByText("Lỗi AI");
    expect(events()).toEqual([]);
  });

  // Scenario: Streamed text whose final message has no cards reports nothing
  it("streamed text without cards reports nothing", async () => {
    vi.stubGlobal("fetch", streamingFetch(["Đang ", "trả lời ", "bạn"]));
    let resolveAsk: (v: ReturnType<typeof reply>) => void = () => {};
    vi.mocked(askAssistantAction).mockReturnValue(
      new Promise((r) => {
        resolveAsk = r;
      }) as never,
    );
    render(<AssistantChatView />);

    await ask("hỏi");
    // Streaming patches are on screen, the unary call is still pending.
    await screen.findByText("Đang trả lời bạn");
    expect(events()).toEqual([]);

    resolveAsk(reply([], "Đang trả lời bạn"));
    await waitFor(() => expect(askAssistantAction).toHaveBeenCalled());
    await waitFor(() =>
      expect(screen.queryByText(/AI đang truy vấn/)).not.toBeInTheDocument(),
    );
    expect(events()).toEqual([]);
  });

  it("streamed text then a failed unary call reports nothing", async () => {
    vi.stubGlobal("fetch", streamingFetch(["Một phần"]));
    vi.mocked(askAssistantAction).mockResolvedValue({
      ok: false,
      message: "x",
    } as never);
    render(<AssistantChatView />);
    await ask("hỏi");
    await screen.findByText("Một phần");
    await waitFor(() =>
      expect(screen.queryByText(/AI đang truy vấn/)).not.toBeInTheDocument(),
    );
    expect(events()).toEqual([]);
  });

  // Scenario: Clicking a product card reports accepted with the listing
  it("reports accepted with the card listing", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply([card("lst_9", "Sản phẩm 9")]),
    );
    render(<AssistantChatView />);
    await ask("gợi ý");
    await screen.findByText("Sản phẩm 9");
    await waitFor(() => expect(events("shown")).toHaveLength(1));
    expect(events("accepted")).toEqual([]);

    const view = screen.getByRole("link", { name: "Xem Ngay" });
    expect(view).toHaveAttribute("href", "/listing/lst_9");
    fireEvent.click(view);
    expect(events("accepted")).toEqual([
      { surface: "assistant", action: "accepted", listingId: "lst_9" },
    ]);

    const buy = screen.getByRole("link", { name: "Mua Ngay" });
    expect(buy).toHaveAttribute("href", "/checkout");
    fireEvent.click(buy);
    expect(events("accepted")).toEqual([
      { surface: "assistant", action: "accepted", listingId: "lst_9" },
      { surface: "assistant", action: "accepted", listingId: "lst_9" },
    ]);
    // No edited/dismissed exists on this surface.
    expect(events("edited")).toEqual([]);
    expect(events("dismissed")).toEqual([]);
  });
});
