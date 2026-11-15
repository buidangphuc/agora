import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { AnchorHTMLAttributes, ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { trackEcommerce } from "@/lib/analytics";
import { trackAiInteraction } from "@/lib/analytics/ai";
import { AssistantChatView } from "./AssistantChatView";
import { askAssistantAction } from "./actions";

vi.mock("./actions", () => ({ askAssistantAction: vi.fn() }));
vi.mock("@/lib/analytics/ai", () => ({ trackAiInteraction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));
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

const APOLOGY =
  "Xin lỗi, trợ lý AI đang tạm thời không phản hồi. Vui lòng thử lại.";

function events(action?: string) {
  return vi
    .mocked(trackAiInteraction)
    .mock.calls.map(([surface, a, listingId, provenance]) => ({
      surface,
      action: a,
      listingId,
      provenance,
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

function reply(
  cards: ReturnType<typeof card>[],
  text = "Đây là gợi ý",
  extra: {
    requestId?: string;
    modelVersion?: string;
    suggestedFollowups?: string[];
  } = {},
) {
  return {
    ok: true as const,
    message: "",
    reply: {
      replyText: text,
      productCards: cards,
      suggestedFollowups: extra.suggestedFollowups ?? [],
      requestId: extra.requestId ?? "",
      modelVersion: extra.modelVersion ?? "",
    },
  };
}

function impressionItems() {
  return vi
    .mocked(trackEcommerce)
    .mock.calls.filter(([name]) => name === "view_item_list")
    .flatMap(([, params]) => params?.items ?? []);
}

async function ask(text: string) {
  fireEvent.change(screen.getByPlaceholderText(/Nhập câu hỏi/), {
    target: { value: text },
  });
  fireEvent.click(screen.getByRole("button", { name: /Gửi/ }));
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
    expect(events("shown")[0]).toMatchObject({
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
        message: "",
      } as never);
    render(<AssistantChatView />);

    await ask("xin chào");
    await screen.findByText("Chỉ có chữ");
    await ask("lỗi nhé");
    await screen.findByText(APOLOGY);
    expect(events()).toEqual([]);
    expect(impressionItems()).toEqual([]);
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
    expect(events("accepted")).toMatchObject([
      { surface: "assistant", action: "accepted", listingId: "lst_9" },
    ]);

    const buy = screen.getByRole("link", { name: "Mua Ngay" });
    expect(buy).toHaveAttribute("href", "/checkout");
    fireEvent.click(buy);
    expect(events("accepted")).toMatchObject([
      { surface: "assistant", action: "accepted", listingId: "lst_9" },
      { surface: "assistant", action: "accepted", listingId: "lst_9" },
    ]);
    // No edited/dismissed exists on this surface.
    expect(events("edited")).toEqual([]);
    expect(events("dismissed")).toEqual([]);
  });
  // Scenarios: One call per question and no stream
  it("asks once and never streams", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, body: null });
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply(
        [card("lst_1", "Tai nghe A"), card("lst_2", "Tai nghe B")],
        "Chỉ một câu trả lời",
      ),
    );
    render(<AssistantChatView />);
    await ask("tai nghe dưới 500k");
    await screen.findByText("Chỉ một câu trả lời");
    expect(askAssistantAction).toHaveBeenCalledTimes(1);
    const streamCalls = fetchMock.mock.calls.filter(([url]) =>
      String(url).includes("/api/assistant/stream"),
    );
    expect(streamCalls).toHaveLength(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  // Scenario: A card without a rating shows no rating line
  it("hides the rating line when empty", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply([{ ...card("lst_1", "Sản phẩm không xếp hạng"), ratingText: "" }]),
    );
    const { container } = render(<AssistantChatView />);
    await ask("q");
    await screen.findByText("Sản phẩm không xếp hạng");
    expect(container.textContent).not.toContain("★");
    expect(container.textContent).not.toMatch(/đánh giá/);
    expect(container.textContent).not.toContain("4.5");
  });

  // Scenario: A rated card shows exactly the returned rating text
  it("shows the returned rating text", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply([
        { ...card("lst_1", "Có đánh giá"), ratingText: "4.5/5 (12 đánh giá)" },
      ]),
    );
    render(<AssistantChatView />);
    await ask("q");
    await screen.findByText("Có đánh giá");
    expect(screen.getByText("4.5/5 (12 đánh giá)")).toBeInTheDocument();
  });

  // Scenario: A failed call shows the apology and no cards
  it("shows the apology on failure", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue({
      ok: false,
      message: "",
    } as never);
    render(<AssistantChatView />);
    await ask("q");
    await screen.findByText(APOLOGY);
    expect(screen.queryByRole("link", { name: "Xem Ngay" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Mua Ngay" })).toBeNull();
  });

  // Scenario: A reply without cards is shown as text only
  it("renders a text-only reply", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply([], "Mình chưa tra cứu được sản phẩm phù hợp", {
        suggestedFollowups: ["Gợi ý khác", "Xem khuyến mãi"],
      }),
    );
    render(<AssistantChatView />);
    await ask("q");
    await screen.findByText("Mình chưa tra cứu được sản phẩm phù hợp");
    expect(screen.queryByRole("link", { name: "Xem Ngay" })).toBeNull();
    expect(screen.getByText("Gợi ý khác")).toBeInTheDocument();
    expect(screen.getByText("Xem khuyến mãi")).toBeInTheDocument();
  });

  // Scenario: Card impressions carry the placement and the request id
  it("cards report impressions with provenance", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply(
        [card("lst_1", "Tai nghe A"), card("lst_2", "Tai nghe B")],
        "Gợi ý",
        {
          requestId: "r1",
          modelVersion: "mock:v1|search-v2",
        },
      ),
    );
    render(<AssistantChatView />);
    await ask("q");
    await screen.findByText("Tai nghe A");
    await waitFor(() => expect(impressionItems()).toHaveLength(2));
    const items = impressionItems();
    for (const it of items) {
      expect(it.itemListId).toBe("assistant.suggestions");
      expect(it.placementId).toBe("assistant.suggestions");
      expect(it.impressionId).toBe("r1");
      expect(it.modelVersion).toBe("mock:v1|search-v2");
    }
    expect(items.map((i) => i.index).sort()).toEqual([1, 2]);
  });

  // Scenario: A reply without a request id sends no impression id
  it("omits empty provenance", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply([card("lst_1", "Tai nghe A")], "Gợi ý", {
        requestId: "",
        modelVersion: "",
      }),
    );
    render(<AssistantChatView />);
    await ask("q");
    await screen.findByText("Tai nghe A");
    await waitFor(() => expect(impressionItems()).toHaveLength(1));
    expect(impressionItems()[0].impressionId).toBeUndefined();
    expect(impressionItems()[0].modelVersion).toBeUndefined();
  });

  // Scenario: The assistant accepted click reuses the reply provenance
  it("accepted carries reply provenance", async () => {
    vi.mocked(askAssistantAction).mockResolvedValue(
      reply([card("lst_9", "Sản phẩm 9")], "Gợi ý", {
        requestId: "r1",
        modelVersion: "mock:v1",
      }),
    );
    render(<AssistantChatView />);
    await ask("q");
    await screen.findByText("Sản phẩm 9");
    await waitFor(() => expect(events("shown")).toHaveLength(1));
    const prov = { requestId: "r1", modelVersion: "mock:v1" };
    expect(events("shown")[0].provenance).toEqual(prov);
    fireEvent.click(screen.getByRole("link", { name: "Xem Ngay" }));
    expect(events("accepted")).toEqual([
      {
        surface: "assistant",
        action: "accepted",
        listingId: "lst_9",
        provenance: prov,
      },
    ]);
  });
});
