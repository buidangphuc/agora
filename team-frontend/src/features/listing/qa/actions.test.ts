import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { answerQuestion, askQuestion } from "@/lib/gateway/engagement";

import { answerQuestionAction, askQuestionAction } from "./actions";

vi.mock("@/lib/gateway/engagement", () => ({
  askQuestion: vi.fn(),
  answerQuestion: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("askQuestionAction", () => {
  it("asks the question (trimmed), revalidates, and returns success", async () => {
    vi.mocked(askQuestion).mockResolvedValue({} as never);
    const res = await askQuestionAction("l1", "  còn bảo hành không?  ");
    expect(askQuestion).toHaveBeenCalledWith("l1", "còn bảo hành không?");
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(res).toEqual({ ok: true, message: "Đã gửi câu hỏi tới shop!" });
  });

  it("returns the error message when the gateway throws", async () => {
    vi.mocked(askQuestion).mockRejectedValue(new Error("rate limited"));
    const res = await askQuestionAction("l1", "hello");
    expect(res).toEqual({ ok: false, message: "rate limited" });
  });
});

describe("answerQuestionAction", () => {
  it("answers with is_shop_reply=true by default and revalidates", async () => {
    vi.mocked(answerQuestion).mockResolvedValue({} as never);
    const res = await answerQuestionAction("l1", "q1", "  còn ạ  ");
    expect(answerQuestion).toHaveBeenCalledWith("q1", "còn ạ", true);
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(res).toEqual({ ok: true, message: "Đã gửi câu trả lời!" });
  });

  it("returns the error message when answering fails", async () => {
    vi.mocked(answerQuestion).mockRejectedValue(new Error("nope"));
    const res = await answerQuestionAction("l1", "q1", "hi");
    expect(res).toEqual({ ok: false, message: "nope" });
  });
});
