import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { listQuestionsByListing } from "@/lib/gateway/engagement";
import { QuestionList } from "./QuestionList";

vi.mock("./actions", () => ({
  askQuestionAction: vi.fn(),
  answerQuestionAction: vi.fn(),
}));
vi.mock("@/lib/gateway/engagement", () => ({
  listQuestionsByListing: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("QuestionList", () => {
  it("shows qa-empty with an ask action (logged in) or a login action (guest)", async () => {
    vi.mocked(listQuestionsByListing).mockResolvedValue([]);
    const { unmount } = render(
      await QuestionList({ listingId: "L", loggedIn: true }),
    );
    const empty = screen.getByTestId("qa-empty");
    expect(empty).toHaveTextContent("Chưa có câu hỏi nào");
    expect(
      within(empty).getByRole("link", { name: "Đặt câu hỏi" }),
    ).toHaveAttribute("href", "#qa-ask");
    unmount();
    render(await QuestionList({ listingId: "L", loggedIn: false }));
    expect(
      within(screen.getByTestId("qa-empty")).getByRole("link", {
        name: "Đăng nhập để hỏi",
      }),
    ).toHaveAttribute("href", "/login?returnUrl=/listing/L");
  });

  it("lists questions and answers with a Shop tag on shop replies and the answer toggle", async () => {
    vi.mocked(listQuestionsByListing).mockResolvedValue([
      {
        id: "q1",
        listingId: "L",
        userId: "u1",
        questionText: "Còn bảo hành không?",
        createdAt: "",
        answers: [
          {
            id: "a1",
            questionId: "q1",
            userId: "s",
            answerText: "Còn ạ",
            isShopReply: true,
            createdAt: "",
          },
          {
            id: "a2",
            questionId: "q1",
            userId: "u2",
            answerText: "Mình cũng hỏi",
            isShopReply: false,
            createdAt: "",
          },
        ],
      },
    ]);
    render(
      <ToastProvider>
        {await QuestionList({ listingId: "L", loggedIn: true })}
      </ToastProvider>,
    );
    const item = screen.getByTestId("qa-item");
    expect(item).toHaveTextContent("Còn bảo hành không?");
    const answers = within(item).getAllByTestId("qa-answer");
    expect(answers).toHaveLength(2);
    expect(within(answers[0]).getByText("Shop")).toBeInTheDocument();
    expect(within(answers[1]).queryByText("Shop")).toBeNull();
    expect(within(item).getByTestId("qa-answer-toggle")).toBeInTheDocument();
  });
});
