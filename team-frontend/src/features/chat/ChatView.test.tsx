import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { AnchorHTMLAttributes, ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { MessageType } from "@/generated/platform/chat/v1/chat_pb.js";
import { trackAiInteraction } from "@/lib/analytics/ai";
import type { ViewChatMessage, ViewChatThread } from "@/lib/gateway/chat";
import { ChatView } from "./ChatView";
import {
  chatCopilotAction,
  fetchThreadMessagesAction,
  listQuickRepliesAction,
  markThreadReadAction,
  searchMessagesAction,
  sendMessageAction,
} from "./actions";

vi.mock("./actions", () => ({
  chatCopilotAction: vi.fn(),
  fetchThreadMessagesAction: vi.fn(),
  listQuickRepliesAction: vi.fn(),
  markThreadReadAction: vi.fn(),
  searchMessagesAction: vi.fn(),
  sendMessageAction: vi.fn(),
}));
vi.mock("@/lib/analytics/ai", () => ({ trackAiInteraction: vi.fn() }));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
  }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    href: string;
    children?: ReactNode;
  }) => <a href={href}>{children}</a>,
}));

const SELLER = "seller_1";
const BUYER = "buyer_1";

const thread: ViewChatThread = {
  id: "t1",
  buyerId: BUYER,
  sellerId: SELLER,
  listingId: "lst_7",
  listingTitle: "Điện thoại",
  listingImageUrl: "",
  lastMessageText: "Còn hàng không?",
  lastMessageAt: "",
  unreadCountBuyer: 0,
  unreadCountSeller: 0,
  createdAt: "",
  updatedAt: "",
};

const messages: ViewChatMessage[] = [
  {
    id: "m1",
    threadId: "t1",
    senderId: BUYER,
    senderName: "Khách",
    content: "Còn hàng không?",
    createdAt: "10:00",
    messageType: MessageType.TEXT,
    listingId: "",
    payload: "",
  },
];

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

function renderAs(userId: string) {
  return render(
    <ChatView
      threads={[thread]}
      activeThread={thread}
      initialMessages={messages}
      currentUserId={userId}
    />,
  );
}

const composer = () =>
  screen.getByPlaceholderText("Nhập nội dung tin nhắn trao đổi...");
const type = (value: string) =>
  fireEvent.change(composer(), { target: { value } });

async function loadSuggestions(replies = ["Gợi ý A", "Gợi ý B", "Gợi ý C"]) {
  vi.mocked(chatCopilotAction).mockResolvedValueOnce({ ok: true, replies });
  fireEvent.click(screen.getByRole("button", { name: /Gợi ý AI/ }));
  await screen.findByRole("button", { name: replies[0] });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(markThreadReadAction).mockResolvedValue(undefined);
  vi.mocked(fetchThreadMessagesAction).mockResolvedValue([]);
  vi.mocked(listQuickRepliesAction).mockResolvedValue([]);
  vi.mocked(searchMessagesAction).mockResolvedValue([]);
  vi.mocked(sendMessageAction).mockResolvedValue({ ok: true } as never);
});

describe("ChatView chat_copilot ai_interaction", () => {
  // Scenario: Suggestions render and are reported once
  it("reports shown once per copilot load", async () => {
    renderAs(SELLER);
    // Default chips before any load are not an AI result.
    expect(events()).toEqual([]);

    await loadSuggestions();
    await waitFor(() => expect(events("shown")).toHaveLength(1));
    expect(events("shown")[0]).toEqual({
      surface: "chat_copilot",
      action: "shown",
      listingId: "lst_7",
    });

    type("x");
    type("xy");
    expect(events("shown")).toHaveLength(1);
  });

  // Scenario: Inserting a suggestion reports accepted
  it("reports accepted on insert", async () => {
    renderAs(SELLER);
    await loadSuggestions();
    fireEvent.click(screen.getByRole("button", { name: "Gợi ý B" }));
    expect(composer()).toHaveValue("Gợi ý B");
    expect(events("accepted")).toEqual([
      { surface: "chat_copilot", action: "accepted", listingId: "lst_7" },
    ]);
    expect(events("edited")).toEqual([]);
  });

  // Scenario: Changing the inserted suggestion reports edited once
  it("reports edited once and only when changed", async () => {
    renderAs(SELLER);
    await loadSuggestions();
    fireEvent.click(screen.getByRole("button", { name: "Gợi ý A" }));
    type("Gợi ý Aa");
    type("Gợi ý Aab");
    type("Gợi ý Aabc");
    expect(events("edited")).toEqual([
      { surface: "chat_copilot", action: "edited", listingId: "lst_7" },
    ]);
  });

  it("reports edited once and only when changed: sending as-is queues no edited", async () => {
    renderAs(SELLER);
    await loadSuggestions();
    fireEvent.click(screen.getByRole("button", { name: "Gợi ý A" }));
    // Re-typing the identical value is not a change.
    type("Gợi ý A");
    fireEvent.click(screen.getByRole("button", { name: /Gửi/ }));
    await waitFor(() => expect(sendMessageAction).toHaveBeenCalled());
    expect(events("edited")).toEqual([]);
  });

  // Scenario: Inserting a second suggestion reports a second accepted and re-arms edited
  it("second insert re-arms edited", async () => {
    renderAs(SELLER);
    await loadSuggestions();
    fireEvent.click(screen.getByRole("button", { name: "Gợi ý A" }));
    type("Gợi ý A!");
    expect(events("edited")).toHaveLength(1);

    fireEvent.click(screen.getByRole("button", { name: "Gợi ý B" }));
    expect(events("accepted")).toHaveLength(2);
    expect(events("edited")).toHaveLength(1);
    type("Gợi ý B!");
    type("Gợi ý B!!");
    expect(events("accepted")).toHaveLength(2);
    expect(events("edited")).toHaveLength(2);
  });

  // Scenario: No suggestions or a failed load reports nothing
  it("reports nothing for empty or failed copilot", async () => {
    renderAs(SELLER);
    vi.mocked(chatCopilotAction).mockResolvedValueOnce({
      ok: true,
      replies: [],
    });
    fireEvent.click(screen.getByRole("button", { name: /Gợi ý AI/ }));
    await waitFor(() => expect(chatCopilotAction).toHaveBeenCalledTimes(1));
    await screen.findByRole("button", { name: /Gợi ý AI/ });

    vi.mocked(chatCopilotAction).mockResolvedValueOnce({
      ok: false,
      replies: [],
      message: "lỗi",
    });
    fireEvent.click(screen.getByRole("button", { name: /Gợi ý AI/ }));
    await waitFor(() => expect(chatCopilotAction).toHaveBeenCalledTimes(2));
    await screen.findByRole("button", { name: /Gợi ý AI/ });
    expect(events()).toEqual([]);
  });

  // Scenario: Not the owner of the thread, no copilot and no copilot events
  it("buyer quick replies emit nothing", async () => {
    vi.mocked(listQuickRepliesAction).mockResolvedValue(["Shop trả lời nhanh"]);
    renderAs(BUYER);
    expect(screen.queryByText(/Gợi ý AI/)).not.toBeInTheDocument();
    fireEvent.click(
      await screen.findByRole("button", { name: "Shop trả lời nhanh" }),
    );
    expect(composer()).toHaveValue("Shop trả lời nhanh");
    type("Shop trả lời nhanh và thêm");
    expect(events()).toEqual([]);
  });
});
